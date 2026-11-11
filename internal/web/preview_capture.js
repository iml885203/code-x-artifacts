(() => {
  // Preview-only, deliberately separate from the viewer's privileged bridge.
  let validLocale = true;
  const locale = new URL(location.href).searchParams.get("lang");
  if (locale === "en" || locale === "zh-TW") {
    try {
      Object.defineProperty(navigator, "language", { value: locale });
      Object.defineProperty(navigator, "languages", { value: Object.freeze([locale]) });
      document.documentElement.lang = locale;
    } catch { validLocale = false; }
  }
  const rasterize = globalThis.htmlToImage?.toSvg;
  const maxBytes = 2 << 20;
  const scrollbarCSS = "*,*::before,*::after{scrollbar-width:none!important}*::-webkit-scrollbar{display:none!important;width:0!important;height:0!important}";
  const hideBars = document.createElement("style");
  hideBars.textContent = scrollbarCSS;
  (document.head || document.documentElement).append(hideBars);
  // An authored selector (or inline rule) may be more specific than the
  // stylesheet. Keep this preview-only inline declaration on new/changed nodes.
  const hideOn = (element) => {
    if (!element.style) return;
    if (element.style.getPropertyValue("scrollbar-width") !== "none" || element.style.getPropertyPriority("scrollbar-width") !== "important") {
      element.style.setProperty("scrollbar-width", "none", "important");
    }
  };
  const hideWithin = (element) => {
    hideOn(element);
    for (const child of element.querySelectorAll("*")) hideOn(child);
  };
  hideWithin(document.documentElement);
  new MutationObserver((records) => {
    for (const record of records) {
      if (record.type === "attributes") hideOn(record.target);
      for (const node of record.addedNodes) if (node.nodeType === Node.ELEMENT_NODE) hideWithin(node);
    }
  }).observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeFilter: ["style"] });
  const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const bounded = (promise, ms) => Promise.race([promise, pause(ms).then(() => { throw Error("not ready"); })]);
  const visible = (el) => el.getClientRects().length > 0 && getComputedStyle(el).visibility !== "hidden";
  const busy = () => [...document.querySelectorAll('progress,[role="progressbar"],[aria-busy="true"],[data-loading="true"],[data-state="loading"]')].some(visible);

  async function settled() {
    await bounded(document.fonts.ready, 2000);
    const pictures = [...document.images].filter(visible);
    await bounded(Promise.all(pictures.map((image) => image.complete ? Promise.resolve() : new Promise((resolve) => {
      image.addEventListener("load", resolve, { once: true });
      image.addEventListener("error", resolve, { once: true });
    }))), 2000);
    let changed = performance.now();
    const observer = new MutationObserver(() => { changed = performance.now(); });
    observer.observe(document.documentElement, { childList: true, subtree: true, attributes: true, characterData: true });
    const start = performance.now();
    try {
      while (performance.now() - start < 5000) {
        if (!busy() && performance.now() - changed >= 400) break;
        await pause(100);
      }
      if (busy() || performance.now() - changed < 400) throw Error("not settled");
      if (pictures.some((image) => image.currentSrc && image.naturalWidth === 0)) throw Error("picture not ready");
      if (!document.body || (!document.body.textContent.trim() && document.body.childElementCount === 0)) throw Error("empty");
      const root = document.getElementById("root");
      if (root && !root.textContent.trim() && root.childElementCount === 0) throw Error("empty root");
    } finally { observer.disconnect(); }
  }

  function childSnapshot(frame) {
    return new Promise((resolve, reject) => {
      const source = frame.contentWindow;
      if (!source) { reject(Error("no frame")); return; }
      const request = [...crypto.getRandomValues(new Uint32Array(4))].join("-");
      const done = (event) => {
        if (event.source !== source || event.data?.pictorPreview !== "snapshot" || event.data.request !== request) return;
        clearTimeout(timer);
        removeEventListener("message", done);
        if (typeof event.data.html !== "string" || event.data.html.length > maxBytes || event.data.failed) reject(Error("uncapturable frame"));
        else resolve(event.data.html);
      };
      const timer = setTimeout(() => { removeEventListener("message", done); reject(Error("frame not ready")); }, 8000);
      addEventListener("message", done);
      source.postMessage({ pictorPreview: "capture", request }, "*");
    });
  }

  async function snapshot() {
    await settled();
    const originals = [document.documentElement, ...document.documentElement.querySelectorAll("*")];
    if (originals.length > 20000) throw Error("too many elements");
    if (originals.some((node) => node.shadowRoot || node.matches("video,object,embed") || (node.scrollTop || 0) !== 0 || (node.scrollLeft || 0) !== 0)) throw Error("unsupported state");
    const copy = document.documentElement.cloneNode(true);
    const copies = [copy, ...copy.querySelectorAll("*")];
    const byOriginal = new Map(originals.map((node, index) => [node, copies[index]]));
    const frames = [...document.querySelectorAll("iframe")];
    if (frames.length > 16) throw Error("too many frames");
    const children = await Promise.all(frames.map(childSnapshot));
    for (let i = 0; i < frames.length; i++) {
      const target = byOriginal.get(frames[i]);
      target.removeAttribute("src");
      target.setAttribute("srcdoc", children[i]);
      target.setAttribute("sandbox", "");
    }
    for (const source of originals) {
      const target = byOriginal.get(source);
      // Blob URLs are tied to the live document; leave this preview live instead.
      if ([...source.attributes].some((attr) => /blob:/i.test(attr.value))) throw Error("ephemeral resource");
      if (source instanceof HTMLCanvasElement) {
        const image = document.createElement("img");
        for (const attr of target.attributes) image.setAttribute(attr.name, attr.value);
        image.src = source.toDataURL();
        image.width = source.width;
        image.height = source.height;
        target.replaceWith(image);
        byOriginal.set(source, image);
      } else if (source instanceof HTMLInputElement) {
        target.setAttribute("value", source.type === "password" ? "" : source.value);
        target.toggleAttribute("checked", source.checked);
      } else if (source instanceof HTMLTextAreaElement) target.textContent = source.value;
      else if (source instanceof HTMLOptionElement) target.toggleAttribute("selected", source.selected);
      else if (source instanceof HTMLStyleElement && source.sheet) {
        target.textContent = [...source.sheet.cssRules].map((rule) => rule.cssText).join("\n");
        if (/blob:/i.test(target.textContent)) throw Error("ephemeral stylesheet");
      }
    }
    // Preserve the visible frame of CSS animation before disabling replay.
    for (const animation of document.getAnimations()) {
      const source = animation.effect?.target;
      const target = byOriginal.get(source);
      if (!target?.style || typeof animation.effect?.getKeyframes !== "function") continue;
      const computed = getComputedStyle(source);
      for (const keyframe of animation.effect.getKeyframes()) {
        for (const name of Object.keys(keyframe)) {
          if (["offset", "computedOffset", "easing", "composite"].includes(name)) continue;
          const property = name.replace(/[A-Z]/g, (letter) => "-" + letter.toLowerCase());
          target.style.setProperty(property, computed.getPropertyValue(property), "important");
        }
      }
    }
    for (const node of copy.querySelectorAll('script,noscript,base,meta[http-equiv],link[rel="preload"],link[rel="modulepreload"]')) node.remove();
    for (const node of [copy, ...copy.querySelectorAll("*")]) {
      for (const attr of [...node.attributes]) {
        if (/^on/i.test(attr.name) || (/^(href|src|action|formaction|xlink:href)$/i.test(attr.name) && /^\s*javascript:/i.test(attr.value))) node.removeAttribute(attr.name);
      }
    }
    const head = copy.querySelector("head");
    if (!head) throw Error("no head");
    const base = document.createElement("base");
    base.href = document.baseURI;
    head.prepend(base);
    for (const sheet of document.adoptedStyleSheets) {
      const style = document.createElement("style");
      style.textContent = [...sheet.cssRules].map((rule) => rule.cssText).join("\n");
      if (/blob:/i.test(style.textContent)) throw Error("ephemeral stylesheet");
      head.append(style);
    }
    const frozen = document.createElement("style");
    frozen.textContent = scrollbarCSS + "*,*::before,*::after{animation:none!important;transition:none!important}";
    head.append(frozen);
    const html = "<!doctype html>\n" + copy.outerHTML;
    if (new TextEncoder().encode(html).byteLength > maxBytes) throw Error("too large");
    return html;
  }

  async function embeddedFonts() {
    const rules = [];
    const seen = new Set();
    async function readSheet(sheet, base, depth = 0) {
      if (depth > 4) throw Error("too many font imports");
      let list;
      try { list = [...sheet.cssRules]; }
      catch {
        if (!sheet.href || seen.has(sheet.href)) return;
        seen.add(sheet.href);
        const response = await fetch(sheet.href, { credentials: "omit" });
        if (!response.ok) throw Error("stylesheet unavailable");
        const text = await response.text();
        if (text.length > 2 << 20) throw Error("stylesheet too large");
        const parsed = new CSSStyleSheet(); parsed.replaceSync(text); list = [...parsed.cssRules]; base = sheet.href;
      }
      for (const rule of list) {
        if (rule.type === CSSRule.FONT_FACE_RULE) rules.push([rule, base]);
        else if (rule.type === CSSRule.IMPORT_RULE) await readSheet(rule.styleSheet || { href: rule.href }, rule.href, depth + 1);
      }
    }
    for (const sheet of document.styleSheets) await readSheet(sheet, sheet.href || document.baseURI);
    const families = new Set([...document.querySelectorAll("*")].flatMap((node) => getComputedStyle(node).fontFamily.toLowerCase().split(",").map((s) => s.trim().replace(/["']/g, ""))));
    const points = [...new Set([...(document.body.textContent || "")].map((c) => c.codePointAt(0)))];
    const usedRange = (range) => !range || range.split(",").some((part) => {
      const value = part.trim().replace(/^U\+/i, ""); const [lo, hi] = value.split("-");
      const low = parseInt(lo.replace(/\?/g, "0"),16); const high = parseInt((hi || lo).replace(/\?/g,"F"),16);
      return points.some((point) => point >= low && point <= high);
    });
    let bytes = 0; let count = 0; const output = [];
    for (const [rule, base] of rules) {
      if (!families.has(rule.style.fontFamily.toLowerCase().replace(/["']/g,"")) || !usedRange(rule.style.getPropertyValue("unicode-range"))) continue;
      let css = rule.cssText;
      for (const match of [...css.matchAll(/url\(["']?([^"')]+)["']?\)/g)]) {
        if (match[1].startsWith("data:")) continue;
        if (++count > 96) throw Error("too many fonts");
        const response = await fetch(new URL(match[1],base), { credentials: "omit" });
        if (!response.ok) throw Error("font unavailable");
        const blob = await response.blob(); bytes += blob.size;
        if (bytes > 16 << 20) throw Error("fonts too large");
        const data = await new Promise((resolve,reject) => { const reader = new FileReader(); reader.onload = () => resolve(reader.result); reader.onerror = reject; reader.readAsDataURL(blob); });
        css = css.replace(match[0], 'url("' + data + '")');
      }
      output.push(css);
    }
    return output.join("\n");
  }

  // Rasterization stays inside the same opaque origin, without a privileged bridge.
  async function bitmap(width, height, outputWidth = width, outputHeight = height) {
    if (!validLocale || !rasterize || ![width,height,outputWidth,outputHeight].every((value) => Number.isInteger(value) && value >= 1 && value <= 2000)) throw Error("unsupported capture");
    await settled();
    const nodes = [...document.querySelectorAll("*")];
    if (nodes.length > 20000 || nodes.some((node) => node.shadowRoot || node.matches("video,object,embed") || (node.scrollTop || 0) !== 0 || (node.scrollLeft || 0) !== 0)) throw Error("unsupported content");
    const frames = [...document.querySelectorAll("iframe")];
    if (frames.length > 16) throw Error("too many frames");
    const replacements = [];
    try {
      for (const frame of frames) {
        const image = await new Promise((resolve, reject) => {
          const source = frame.contentWindow;
          const request = crypto.randomUUID();
          const done = (event) => {
            if (event.source !== source || event.origin !== "null" || event.data?.pictorPreview !== "thumbnail" || event.data.request !== request) return;
            clearTimeout(timer); removeEventListener("message", done);
            if (typeof event.data.image !== "string" || !event.data.image.startsWith("data:image/jpeg;base64,") || event.data.image.length > maxBytes) reject(Error("frame capture failed"));
            else resolve(event.data.image);
          };
          const timer = setTimeout(() => { removeEventListener("message", done); reject(Error("frame timeout")); }, 12000);
          addEventListener("message", done);
          source.postMessage({ pictorPreview: "thumbnail-capture", request, width: frame.clientWidth, height: frame.clientHeight }, "*");
        });
        const replacement = document.createElement("img");
        for (const attr of frame.attributes) if (attr.name !== "src" && attr.name !== "srcdoc" && attr.name !== "sandbox") replacement.setAttribute(attr.name, attr.value);
        const computed = getComputedStyle(frame);
        for (const property of computed) replacement.style.setProperty(property, computed.getPropertyValue(property));
        replacement.src = image;
        replacement.style.width = frame.offsetWidth + "px"; replacement.style.height = frame.offsetHeight + "px";
        replacements.push([frame, replacement]); frame.replaceWith(replacement);
        await replacement.decode();
      }
      for (const original of document.querySelectorAll("canvas")) {
        const image = document.createElement("img");
        for (const attr of original.attributes) image.setAttribute(attr.name, attr.value);
        const computed = getComputedStyle(original);
        for (const property of computed) image.style.setProperty(property, computed.getPropertyValue(property));
        image.src = original.toDataURL(); image.style.width = original.clientWidth + "px"; image.style.height = original.clientHeight + "px";
        replacements.push([original, image]); original.replaceWith(image); await image.decode();
      }
      const missingResource = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a0xkAAAAASUVORK5CYII=#pictor-thumbnail-missing";
      const svg = await rasterize(document.documentElement, {
        width, height, canvasWidth: width, canvasHeight: height, pixelRatio: 1,
        backgroundColor: getComputedStyle(document.documentElement).backgroundColor === "rgba(0, 0, 0, 0)" ? "#ffffff" : getComputedStyle(document.documentElement).backgroundColor,
        imagePlaceholder: missingResource, includeQueryParams: true, fontEmbedCSS: await embeddedFonts(), preferredFontFormat: "woff2", fetchRequestInit: { credentials: "omit" },
        filter: (node) => !["SCRIPT", "NOSCRIPT", "HEAD"].includes(node.nodeName),
      });
      const markup = decodeURIComponent(svg).replace(/&quot;/g, '"');
      if (markup.includes(missingResource) || /url\(\s*["']?(?:https?:|blob:)/i.test(markup)) throw Error("resource embedding failed");
      const image = new Image(); image.src = svg; await image.decode();
      const canvas = document.createElement("canvas"); canvas.width = outputWidth; canvas.height = outputHeight;
      const ctx = canvas.getContext("2d"); ctx.fillStyle = "#fff"; ctx.fillRect(0,0,outputWidth,outputHeight); ctx.drawImage(image,0,0,outputWidth,outputHeight);
      return canvas.toDataURL("image/jpeg", 0.82);
    } finally { for (const [frame, replacement] of replacements) replacement.replaceWith(frame); }
  }

  let capturing = false;
  addEventListener("message", async (event) => {
    const data = event.data;
    if (event.source !== parent || !["capture", "thumbnail-capture"].includes(data?.pictorPreview) || typeof data.request !== "string" || data.request.length > 128 || capturing) return;
    capturing = true;
    const thumbnail = data.pictorPreview === "thumbnail-capture";
    try {
      if (thumbnail) {
        const image = await bounded(bitmap(data.width, data.height, data.outputWidth, data.outputHeight), 20000);
        parent.postMessage({ pictorPreview: "thumbnail", request: data.request, image }, "*");
      } else parent.postMessage({ pictorPreview: "snapshot", request: data.request, html: await snapshot() }, "*");
    } catch { parent.postMessage({ pictorPreview: thumbnail ? "thumbnail" : "snapshot", request: data.request, failed: true }, "*"); }
    finally { capturing = false; }
  });
})();
