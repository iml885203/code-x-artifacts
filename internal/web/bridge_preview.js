(() => {
  // A card on the overview shows the page, nothing more: window.pictor has the
  // shape pages expect, and every call waits for an answer no one will give, so
  // a page that asks for its data keeps showing what it shows before it has it.
  const never = () => new Promise(() => {});
  window.pictor = Object.freeze({
    whoami: never,
    download: never,
    db: Object.freeze({ get: never, set: never, update: never, delete: never, list: never, query: never, batch: never }),
  });
})();
