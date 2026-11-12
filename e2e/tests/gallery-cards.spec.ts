import { expect, publish, republish, signIn, test } from "./support";

test("a card's time moves from now to a minute ago without a reload", async ({ page }) => {
  await signIn(page, "alice");
  await publish(page, { title: "Clock card", content: "<!doctype html><p>x</p>" });
  // The page's clock is faked from here on, so the minute is run, not waited for.
  await page.clock.install({ time: new Date() });
  await page.goto("/");
  const card = page.locator("li.card", { hasText: "Clock card" });
  await expect(card.locator("time")).toHaveText("Edited now");
  await page.clock.runFor(61_000);
  await expect(card.locator("time")).toHaveText("Edited 1 minute ago");
  // The exact time stays available to a pointer and to assistive technology.
  const time = card.locator("time");
  await time.hover();
  await page.clock.runFor(400);
  await expect(page.getByRole("tooltip")).toHaveText(/^\d{4}-\d\d-\d\d \d\d:\d\d$/);
  await expect(time).toHaveAttribute("aria-describedby", await page.getByRole("tooltip").getAttribute("id") ?? "");
  await expect(time).not.toHaveAttribute("title");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("tooltip")).toHaveCount(0);
  await time.focus();
  await expect(page.getByRole("tooltip")).toBeVisible();
});

test("a card says who can open it with an icon and wears a badge for its type, and its corner actions show on hover, focus and touch", async ({ page, browser }) => {
  await signIn(page, "alice");
  const { id } = await publish(page, { title: "Badge card", content: "<!doctype html><p>x</p>", visibility: "organisation" });
  await page.goto("/");
  const card = page.locator(`li.card[data-artifact-id="${id}"]`);
  await expect(card.getByRole("img", { name: "Organisation" })).toBeVisible();
  await expect(card.getByRole("img", { name: "Web page" })).toBeVisible();
  const menuButton = card.getByRole("button", { name: "Actions for Badge card" });
  const pinButton = card.getByRole("button", { name: "Pin Badge card" });
  await expect(menuButton).toHaveCSS("opacity", "0");
  // The keyboard reaches the group without a pointer: focus on the card's link shows it.
  await card.getByRole("link").first().focus();
  await expect(menuButton).toHaveCSS("opacity", "1");
  await expect(pinButton).toHaveCSS("opacity", "1");
  await page.evaluate(() => { if (document.activeElement instanceof HTMLElement) document.activeElement.blur(); });
  await page.mouse.move(0, 0);
  await expect(menuButton).toHaveCSS("opacity", "0");
  await card.hover();
  await expect(menuButton).toHaveCSS("opacity", "1");
  // The group sits at the card's upper right, over the preview.
  const cardBox = await card.boundingBox(); // page-level
  const groupBox = await card.locator(".card-actions").boundingBox(); // page-level
  expect(groupBox && cardBox && groupBox.x + groupBox.width > cardBox.x + cardBox.width - 24 && groupBox.y < cardBox.y + 24).toBe(true);
  // Pin and More are one group in the preview's corner, beside the open link, and More opens its menu without opening the artifact.
  const group = card.locator(".card-actions");
  await expect(group.getByRole("button")).toHaveCount(2);
  await expect(card.locator("a.card-link .card-actions")).toHaveCount(0);
  const url = page.url();
  await menuButton.click();
  await expect(page.getByRole("menuitem", { name: "Delete…" })).toBeVisible();
  expect(page.url()).toBe(url);
  await page.keyboard.press("Escape");
  await expect(menuButton).toBeFocused();
  await pinButton.click();
  await page.evaluate(() => { if (document.activeElement instanceof HTMLElement) document.activeElement.blur(); });
  await page.mouse.move(0, 0);
  const pinnedButton = card.getByRole("button", { name: "Unpin Badge card" });
  await expect(pinnedButton).toHaveCSS("opacity", "1");
  await expect(menuButton).toHaveCSS("opacity", "0");
  const pinnedBox = await pinnedButton.boundingBox(); // page-level
  const settledCard = await card.boundingBox(); // page-level
  expect(pinnedBox && settledCard && settledCard.x + settledCard.width - pinnedBox.x - pinnedBox.width <= 10).toBe(true);
  await card.hover();
  await pinnedButton.click();

  // On a phone with no hover the same corner actions stay visible and never open the artifact.
  const phone = await browser.newContext({ baseURL: page.url().split("/").slice(0, 3).join("/"), viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });
  const touch = await phone.newPage();
  // Half an hour on, the card says "Edited 30 minutes ago": the longest time line a card carries.
  await touch.clock.install({ time: new Date(Date.now() + 30 * 60_000) });
  await signIn(touch, "alice");
  const phoneCard = touch.locator(`li.card[data-artifact-id="${id}"]`);
  const phonePreview = await phoneCard.locator(".thumb").boundingBox(); // page-level
  const phoneCardBox = await phoneCard.boundingBox(); // page-level
  expect(phonePreview && phoneCardBox && phonePreview.width >= phoneCardBox.width - 2 && phonePreview.height > 100).toBe(true);
  const phoneMenu = phoneCard.getByRole("button", { name: "Actions for Badge card" });
  await expect(phoneMenu).toHaveCSS("opacity", "1");
  await expect(phoneCard.getByRole("button", { name: "Pin Badge card" })).toHaveCSS("opacity", "1");
  // Each control is drawn at 32px and takes a touch anywhere in the 44px square around it.
  for (const target of [phoneMenu, phoneCard.getByRole("button", { name: "Pin Badge card" })]) {
    const box = await target.boundingBox(); // page-level
    expect(box && box.width >= 32 && box.height >= 32).toBe(true);
    await target.scrollIntoViewIfNeeded();
    // Polled: while the page's entrance view transition runs, every point hits the transition, not the page.
    await expect.poll(() => target.evaluate((b) => {
      const r = b.getBoundingClientRect();
      const [cx, cy] = [r.x + r.width / 2, r.y + r.height / 2];
      return [[0, 0], [-21, 0], [21, 0], [0, -21], [0, 21]].filter(([dx = 0, dy = 0]) => !b.contains(document.elementFromPoint(cx + dx, cy + dy)));
    })).toEqual([]);
  }
  // The author's name is whole beside the time, not cut to a letter.
  await expect(phoneCard.locator("time")).toHaveText("Edited 30 minutes ago");
  const author = await phoneCard.locator(".card-author-name").evaluate((n) => ({ text: n.textContent, shown: n.clientWidth, needs: n.scrollWidth }));
  expect(author.text).toBe("ALICE");
  expect(author.shown).toBeGreaterThan(0);
  expect(author.shown).toBeGreaterThanOrEqual(author.needs);
  const phoneBox = await phoneCard.boundingBox(); // page-level
  const phoneGroup = await phoneCard.locator(".card-actions").boundingBox(); // page-level
  expect(phoneBox && phoneGroup && phoneGroup.y < phoneBox.y + 24 && phoneGroup.x + phoneGroup.width > phoneBox.x + phoneBox.width - 24).toBe(true);
  const phoneUrl = touch.url();
  await phoneMenu.tap();
  await expect(touch.getByRole("menuitem", { name: "Delete…" })).toBeVisible();
  expect(touch.url()).toBe(phoneUrl);
  await touch.keyboard.press("Escape");
  await phoneCard.getByRole("button", { name: "Pin Badge card" }).tap();
  await expect(phoneCard.getByRole("button", { name: "Unpin Badge card" })).toBeVisible();
  const pinned = await phoneCard.getByRole("button", { name: "Unpin Badge card" }).boundingBox(); // page-level
  const pinnedCard = await phoneCard.boundingBox(); // page-level
  expect(pinned && pinnedCard && pinnedCard.x + pinnedCard.width - pinned.x - pinned.width <= 10).toBe(true);
  expect(touch.url()).toBe(phoneUrl);
  await phoneCard.getByRole("button", { name: "Unpin Badge card" }).tap();
  await expect(phoneCard.getByRole("button", { name: "Pin Badge card" })).toBeVisible();
  await phone.close();
});

test("carol finds a card by search, pins it with P and renames it with R from its menu, they show under Pinned and survive a reload; she unpins it from the viewer", async ({ page }) => {
  await signIn(page, "carol");
  const ids: Record<string, string> = {};
  for (const title of ["Pin one", "Pin two"]) ids[title] = (await publish(page, { title, content: "<!doctype html><p>x</p>" })).id;
  await page.goto("/");
  const headings = page.locator("li.card h2");
  // Both were made in the same second, so which is listed first is the server's call: use the one listed second.
  await expect(headings.filter({ hasText: /^Pin (one|two)$/ })).toHaveCount(2);
  const [upper, lower] = await headings.filter({ hasText: /^Pin (one|two)$/ }).allTextContents();
  const target = lower ?? "";
  expect(upper).not.toBe(target);
  await expect(headings.first()).not.toHaveText(target);

  // Search narrows the wall to the card; its menu pins it and then renames it, each with its letter.
  await page.getByRole("searchbox", { name: "Search" }).fill(target);
  await expect(headings).toHaveText([target]);
  const card = page.locator("li.card", { hasText: target });
  await card.hover();
  await card.getByRole("button", { name: `Actions for ${target}` }).click();
  await page.keyboard.press("p");
  await expect(card.getByRole("button", { name: `Unpin ${target}` })).toBeVisible();
  await card.hover();
  await card.getByRole("button", { name: `Actions for ${target}` }).click();
  await page.keyboard.press("r");
  const renamed = `${target} renamed`;
  await page.getByRole("textbox", { name: "Title" }).fill(renamed);
  await page.keyboard.press("Enter");
  await expect(headings).toHaveText([renamed]);

  await page.getByRole("searchbox", { name: "Search" }).clear();
  await expect(headings.first()).toHaveText(renamed);
  await page.getByRole("radio", { name: /^Pinned/ }).click();
  await expect(headings).toHaveText([renamed]);

  // The pin and the name are carol's and the server's: both survive a reload.
  await page.reload();
  await page.getByRole("radio", { name: /^Pinned/ }).click();
  await expect(headings).toHaveText([renamed]);

  await page.goto(`/a/${ids[target] ?? ""}`);
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "Unpin" }).click();
  await page.goto("/");
  await expect(page.getByRole("radio", { name: /^Pinned/ })).toHaveText("Pinned0");
  await expect(page.locator("li.card", { hasText: renamed }).getByRole("button", { name: `Pin ${renamed}` })).toBeVisible();
});

test("carol does not see bob's pin, groups the library by owner as a list, and finds bob's page first under Recently opened once she opened it", async ({ page, browser }) => {
  await signIn(page, "bob");
  const a = await publish(page, { title: "Bob pins this", content: "<!doctype html><p>x</p>", visibility: "organisation" });
  const earlier = await publish(page, { title: "Bob's earlier page", content: "<!doctype html><p>x</p>", visibility: "organisation" });
  await page.goto("/");
  const mine = page.locator(`li.card[data-artifact-id="${a.id}"]`);
  await mine.getByRole("button", { name: "Pin Bob pins this" }).click();
  await expect(mine.getByRole("button", { name: "Unpin Bob pins this" })).toBeVisible();
  const other = await browser.newContext({ baseURL: page.url().split("/").slice(0, 3).join("/") });
  const carol = await other.newPage();
  await signIn(carol, "carol");
  const recorded = carol.waitForResponse((r) => r.url().endsWith(`/api/artifacts/${earlier.id}/open`) && r.ok());
  await carol.goto(`/a/${earlier.id}`);
  await recorded;
  await carol.goto("/");
  await expect(carol.locator(`li.card[data-artifact-id="${a.id}"]`).getByRole("button", { name: "Pin Bob pins this" })).toBeVisible();

  // Grouped by owner and shown as a list, bob's item sits under his name; both choices are carol's and outlive a reload.
  await carol.getByRole("button", { name: /^Group by/ }).click();
  await carol.getByRole("menuitemradio", { name: "Owner" }).click();
  await carol.getByRole("button", { name: "List" }).click();
  const underBob = () => carol.getByRole("region", { name: "BOB", exact: true }).locator(`li.lib-row[data-artifact-id="${a.id}"]`);
  await expect(underBob()).toBeVisible();
  await carol.reload();
  await expect(carol.getByRole("button", { name: "List" })).toHaveAttribute("aria-pressed", "true");
  await expect(underBob()).toBeVisible();

  // Carol opens bob's page, goes back to the library and finds it first under Recently opened, in the same list rows.
  // Other tests sign in as carol at the same time and open pages of their own, so "first" is among the pages this test opened.
  await underBob().getByRole("link").first().click();
  await expect(carol).toHaveURL(new RegExp(`/a/${a.id}$`));
  await carol.getByRole("link", { name: "Back to artifacts" }).click();
  await carol.getByRole("navigation", { name: "Library" }).getByRole("link", { name: "Recently opened" }).click();
  await expect(carol).toHaveURL(/\/recent$/);
  const ours = [a.id, earlier.id];
  const opened = () => carol.locator("li.lib-row").evaluateAll((rows, ids) => rows.map((r) => r.getAttribute("data-artifact-id")).filter((id) => id !== null && ids.includes(id)), ours);
  await expect.poll(opened).toEqual([a.id, earlier.id]);
  await carol.reload();
  await expect.poll(opened).toEqual([a.id, earlier.id]);
  await other.close();
});

test("gallery cards reuse image thumbnails across formats without loading artifact documents", async ({ page, browser, watch, expectConsoleErrors }) => {
  await signIn(page, "alice");
  const ids: string[] = [];
  const made = async (body: Parameters<typeof publish>[1]) => ids.push((await publish(page, body)).id);
  await made({ title: "Image HTML", content: '<!doctype html><style>body{background:#ffe0e8}.scroller{width:150px;height:80px;overflow:scroll;scrollbar-width:auto!important}</style><h1>HTML thumbnail</h1><p id="thumbnail-ready" aria-busy="true">Preparing capture</p><div class="scroller"><div style="height:800px">Scroll content</div></div>' });
  await made({ title: "Image canvas", content: '<!doctype html><h1>Canvas thumbnail</h1><canvas id=c width=160 height=100></canvas><script>const ctx=document.getElementById("c").getContext("2d");ctx.fillStyle="red";ctx.fillRect(0,0,160,100)</script>' });
  await made({ title: "Image doc", kind: "doc", content: "# Document 縮圖\n\nA paragraph.\n" });
  await made({ title: "Image React", kind: "react", content: 'export default function App(){const dark=matchMedia("(prefers-color-scheme: dark)").matches;return <div style={{height:"900px",background:navigator.language!=="en"?"rgb(0,255,0)":innerWidth<=640?"rgb(255,255,0)":innerWidth<=1024?"rgb(255,0,255)":dark?"rgb(0,0,255)":"rgb(255,0,0)"}}><h1>{dark?"Dark thumbnail":"Light thumbnail"}</h1></div>}' });
  await made({ title: "Image design", kind: "design", content: JSON.stringify({version:1,pages:[{id:"p",name:"Page",rows:[[{id:"f",name:"Frame",file:"frames/f.html",w:400,h:300}]]}]}), files:{"frames/f.html":"<!doctype html><body><h1>Design thumbnail</h1></body>"} });
  await made({ title: "Image board", kind: "board", content: JSON.stringify({version:1,elements:[{id:"box",type:"rectangle",x:0,y:0,label:"Board thumbnail"}]}) });
  await made({ title: "Image slides", kind: "slides", content: JSON.stringify({version:1,slides:[{layout:"title",title:"Slides thumbnail"}]}) });
  const card=(index:number)=>page.locator(`li.card[data-artifact-id="${ids[index]}"]`);
  let finishUpload: (()=>void) | undefined;
  const uploadGate=new Promise<void>((resolve)=>{finishUpload=resolve;});
  await page.route("**/api/artifacts/*/thumbnail?*",async(route)=>{
    if(route.request().method()==="POST"&&new URL(route.request().url()).pathname===`/api/artifacts/${ids[0]}/thumbnail`)await uploadGate;
    await route.continue();
  });
  const pendingUpload=page.waitForRequest((request)=>request.method()==="POST"&&new URL(request.url()).pathname===`/api/artifacts/${ids[0]}/thumbnail`);

  await page.goto("/");
  await card(0).scrollIntoViewIfNeeded();
  try {
    await expect(card(0).locator(".thumb-screen")).toHaveClass(/loaded/);
    await expect(card(0).frameLocator("iframe").getByRole("heading",{name:"HTML thumbnail"})).toBeVisible();
    expect(await card(0).frameLocator("iframe").locator("html").evaluate(()=>innerWidth)).toBeGreaterThanOrEqual(1200);
    await expect(card(0).locator(".thumb-screen > img")).toHaveCount(0);
    await card(0).frameLocator("iframe").locator("#thumbnail-ready").evaluate((node)=>{node.remove();});
    await pendingUpload;
  } finally { finishUpload?.(); }
  for (let i=0;i<ids.length;i++) {
    await card(i).scrollIntoViewIfNeeded();
    await expect(card(i).locator(".thumb-screen > img")).toBeVisible({timeout:25000});
    await expect(card(i).locator(".thumb-screen")).toHaveClass(/loaded/);
    await expect(card(i).locator("iframe")).toHaveCount(0);
    const image=card(i).locator(".thumb-screen > img");
    expect(await image.evaluate((node)=>{const img=node as HTMLImageElement;return img.naturalWidth===1120&&img.naturalHeight===720;})).toBe(true);
  }
  const reactPixel=async()=>card(3).locator(".thumb-screen > img").evaluate((node)=>{const canvas=document.createElement("canvas");canvas.width=1120;canvas.height=720;const context=canvas.getContext("2d");if(!context)throw Error("no canvas");context.drawImage(node as HTMLImageElement,0,0);return [...context.getImageData(1000,500,1,1).data];});
  expect(await reactPixel()).toEqual([254,0,0,255]);
  const warmRequests:string[]=[];
  page.on("request",request=>{const path=new URL(request.url()).pathname;if(request.resourceType()==="document"&&ids.some((id)=>path===`/raw/${id}`||path.startsWith(`/raw/${id}/`)||path===`/preview/${id}`))warmRequests.push(path);});
  await page.reload();
  for(let i=0;i<ids.length;i++){await card(i).scrollIntoViewIfNeeded();await expect(card(i).locator(".thumb-screen.loaded > img")).toBeVisible();}
  expect(warmRequests).toEqual([]);
  // Only opened artifacts have interactive scrolling.
  await card(0).getByRole("link").first().click();
  const scroller=page.frameLocator("iframe[data-artifact-frame]").locator(".scroller");
  await expect(scroller).toHaveCSS("scrollbar-width","auto");
  expect(await scroller.evaluate((node)=>{node.scrollTop=40;return node.scrollTop;})).toBe(40);
  await page.goto("/");
  await card(0).scrollIntoViewIfNeeded();
  await expect(card(0).locator(".thumb-screen.loaded > img")).toBeVisible();
  const previous=await card(0).locator(".thumb-screen > img").getAttribute("src");
  await republish(page,ids[0]??"",{title:"Image HTML",content:"<!doctype html><h1>Updated thumbnail</h1>",baseVersion:1});
  await page.reload();await card(0).scrollIntoViewIfNeeded();
  await expect(card(0).locator(".thumb-screen.loaded > img")).toBeVisible({timeout:25000});
  expect(await card(0).locator(".thumb-screen > img").getAttribute("src")).not.toBe(previous);
  await page.emulateMedia({colorScheme:"dark"});await card(3).scrollIntoViewIfNeeded();
  await expect(card(3).locator(".thumb-screen.loaded > img")).toHaveAttribute("src",/scheme=dark/,{timeout:25000});
  expect(await reactPixel()).toEqual([0,0,254,255]);
  await page.evaluate(()=>{localStorage.setItem("pictor-theme","light");});
  await page.reload();await card(3).scrollIntoViewIfNeeded();
  await expect(card(3).locator(".thumb-screen.loaded > img")).toHaveAttribute("src",/scheme=light/,{timeout:25000});
  expect(await reactPixel()).toEqual([254,0,0,255]);
  await page.evaluate(()=>{localStorage.removeItem("pictor-theme");});
  await page.emulateMedia({colorScheme:"light"});
  await expect(card(3).locator(".thumb-screen.loaded > img")).toHaveAttribute("src",/scheme=light/,{timeout:25000});
  await page.setViewportSize({width:390,height:844});await card(3).scrollIntoViewIfNeeded();
  await expect(card(3).locator(".thumb-screen.loaded > img")).toHaveAttribute("src",/viewport=mobile/,{timeout:25000});
  const mobilePixel=await reactPixel();expect(mobilePixel[0]).toBeGreaterThan(240);expect(mobilePixel[1]).toBeGreaterThan(240);expect(mobilePixel[2]).toBeLessThan(15);
  await page.setViewportSize({width:900,height:900});await card(3).scrollIntoViewIfNeeded();
  await expect(card(3).locator(".thumb-screen.loaded > img")).toHaveAttribute("src",/viewport=tablet/,{timeout:25000});
  const tabletPixel=await reactPixel();expect(tabletPixel[0]).toBeGreaterThan(240);expect(tabletPixel[1]).toBeLessThan(15);expect(tabletPixel[2]).toBeGreaterThan(240);
  await page.setViewportSize({width:1280,height:720});await card(3).scrollIntoViewIfNeeded();
  await expect(card(3).locator(".thumb-screen.loaded > img")).toHaveAttribute("src",/viewport=desktop/,{timeout:25000});
  // A loaded CSS image that cannot be embedded must not become a permanent incomplete thumbnail.
  expectConsoleErrors(/capture-background\.png/);
  // This deliberately broken artifact belongs to a separate actor so other tests' galleries never load it.
  const origin = new URL(page.url()).origin;
  const isolated = await browser.newContext({baseURL:origin});
  const failedPage = watch(await isolated.newPage());
  try {
    await failedPage.goto("/login");
    await failedPage.getByLabel("Username").fill("dana");
    await failedPage.getByLabel("Password").fill("unused-test-password");
    await failedPage.getByRole("button",{name:"Sign in",exact:true}).click();
    await expect(failedPage.getByRole("heading",{name:"Artifacts",exact:true})).toBeVisible();
    await failedPage.route("**/capture-background.png", async (route) => {
      if (route.request().resourceType() === "fetch") { await route.abort("failed"); return; }
      await route.fulfill({contentType:"image/png",body:Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a0xkAAAAASUVORK5CYII=","base64")});
    });
    const failed=await publish(failedPage,{title:"Unembeddable background",content:`<!doctype html><body style="background-image:url('${origin}/capture-background.png')"><h1>Background must be included</h1>`});
    await failedPage.reload();
    const failedCard=failedPage.locator(`li.card[data-artifact-id="${failed.id}"]`);await failedCard.scrollIntoViewIfNeeded();
    await expect(failedCard.locator("iframe")).toHaveCount(1);
    await expect(failedCard.locator("iframe")).toHaveCount(0,{timeout:25000});
    await expect(failedCard.locator(".thumb-screen > img")).toHaveCount(0);
    await expect(failedCard.getByRole("link").first()).toHaveAttribute("href",`/a/${failed.id}`);
  } finally { await isolated.close(); }


});
