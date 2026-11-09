(() => {
  let seq = 0;
  const pending = new Map();
  addEventListener("message", (e) => {
    const m = e.data;
    if (e.source !== parent || !m) return;
    if (m.pictor === "scrollTo") {
      restore(m.x, m.y);
      return;
    }
    // Export, PDF: the viewer asks the page to print itself, once the fonts,
    // pictures and frames it needs are in, so the printout has the page's own
    // print stylesheet and not the viewer's frame around it.
    if (m.pictor === "print") {
      Promise.all([document.fonts.ready, contentLoaded()]).then(() => print());
      return;
    }
    if (m.pictor !== "reply" || !pending.has(m.id)) return;
    const p = pending.get(m.id);
    pending.delete(m.id);
    m.error ? p.reject(new Error(m.error)) : p.resolve(m.result);
  });
  // A picture or frame that waits to come near the screen (loading="lazy") is
  // not loaded when the browser prints, and prints blank. Asked to print, the page
  // loads them all and resolves once each has loaded (or failed). The load event
  // does not bubble but is seen on the way down, so a frame that loaded before
  // this is asked is known too.
  const loaded = new WeakSet();
  document.addEventListener("load", (e) => loaded.add(e.target), true);
  const contentLoaded = () =>
    Promise.all(
      [...document.querySelectorAll("img, iframe")].map((el) => {
        if (el.getAttribute("loading") === "lazy") el.setAttribute("loading", "eager");
        if (loaded.has(el) || (el.localName === "img" && el.complete)) return null;
        return new Promise((done) => {
          el.addEventListener("load", done, { once: true });
          el.addEventListener("error", done, { once: true });
        });
      }),
    );
  // A frame that has just loaded can still have no size (Chrome lays the new
  // frame out after its load event): there is nothing to scroll yet, and
  // scrollTo would be dropped. The place is restored when the frame gets its size.
  const restore = (x, y) => {
    if (innerWidth > 0 && innerHeight > 0) {
      scrollTo(x, y);
      return;
    }
    addEventListener("resize", () => restore(x, y), { once: true });
  };
  // The viewer keeps the place in the page when a new version replaces this frame.
  const framed = parent !== window;
  let reported = false;
  addEventListener(
    "scroll",
    () => {
      if (!framed || reported) return;
      reported = true;
      requestAnimationFrame(() => {
        reported = false;
        parent.postMessage({ pictor: "scroll", x: scrollX, y: scrollY }, "*");
      });
    },
    { passive: true },
  );
  addEventListener("load", () => {
    if (framed) parent.postMessage({ pictor: "ready" }, "*");
  });
  const call = (op, args) =>
    new Promise((resolve, reject) => {
      const id = ++seq;
      pending.set(id, { resolve, reject });
      parent.postMessage({ pictor: "call", id, op, args }, "*");
    });
  // The viewer decides whether a file is saved, from limits of its own, and asks the
  // person first; the page only describes the file. A Blob is read here because the
  // viewer takes text or bytes.
  const download = async (file) => {
    if (!framed) throw new Error("a page can save a file only when it is open in CodeXArtifacts");
    if (!file || typeof file !== "object") return call("download", file);
    const { name, type, data } = file;
    const isBlob = typeof Blob !== "undefined" && data instanceof Blob;
    return call("download", { name, type: type ?? (isBlob ? data.type : undefined), data: isBlob ? await data.arrayBuffer() : data });
  };
  window.pictor = Object.freeze({
    whoami: () => call("whoami"),
    download,
    db: Object.freeze({
      get: (collection, id) => call("db.get", { collection, id }),
      set: (collection, id, data, opts) => call("db.set", { collection, id, data, ifVersion: opts?.ifVersion }),
      update: (collection, id, fields, opts) => call("db.update", { collection, id, fields, ifVersion: opts?.ifVersion }),
      delete: (collection, id, opts) => call("db.delete", { collection, id, ifVersion: opts?.ifVersion }),
      list: (collection) => call("db.list", { collection }),
      query: (collection, filter, opts) => call("db.query", { collection, filter, limit: opts?.limit }),
      batch: (ops) => call("db.batch", { ops }),
    }),
  });
})();
