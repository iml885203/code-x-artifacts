(() => {
  // Preview-only, deliberately separate from the viewer's privileged bridge.
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

  let capturing = false;
  addEventListener("message", async (event) => {
    const data = event.data;
    if (event.source !== parent || data?.pictorPreview !== "capture" || typeof data.request !== "string" || data.request.length > 128 || capturing) return;
    capturing = true;
    try { parent.postMessage({ pictorPreview: "snapshot", request: data.request, html: await snapshot() }, "*"); }
    catch { parent.postMessage({ pictorPreview: "snapshot", request: data.request, failed: true }, "*"); }
    finally { capturing = false; }
  });
})();
