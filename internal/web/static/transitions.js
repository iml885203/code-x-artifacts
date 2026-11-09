// Loaded by the shell as a plain script in <head>, ahead of the app, because
// everything here has to exist before the page is first shown: the browser
// raises `pagereveal` then and aborts the transition of an ordinary navigation
// (the stylesheet's `@view-transition`) when the visit is cut short, with the
// stylesheet not yet applied, or when the page it leaves is a file. The app is
// a deferred module and can run after that, so a listener it registered would
// miss the event, and an aborted transition rejects its promises with no one
// to take them (an uncaught error in the console).
(function () {
  "use strict";

  function quiet(transition) {
    transition.ready.catch(function () {});
    transition.updateCallbackDone.catch(function () {});
    transition.finished.catch(function () {});
  }

  ["pagereveal", "pageswap"].forEach(function (type) {
    window.addEventListener(type, function (e) {
      if (e.viewTransition) quiet(e.viewTransition);
    });
  });

  // A transition the browser aborted before it gave the page the event has no
  // object to attach to, yet it still rejects, and the rejection is nobody's.
  window.addEventListener("unhandledrejection", function (e) {
    var r = e.reason;
    if (r instanceof DOMException && (r.name === "InvalidStateError" || r.name === "AbortError") && r.message.indexOf("Transition was aborted") === 0) {
      e.preventDefault();
    }
  });
})();
