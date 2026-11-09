(function () {
  var deck = document.getElementById("deck");
  var slides = Array.prototype.slice.call(document.querySelectorAll(".slide"));
  var count = slides.length;
  var current = 0;
  var prev = document.getElementById("prev");
  var next = document.getElementById("next");
  var counter = document.getElementById("count");
  var notesButton = document.getElementById("notes-toggle");
  var fullButton = document.getElementById("full-toggle");
  var panel = document.getElementById("notes-panel");
  var bar = document.getElementById("bar");

  function show(index) {
    index = Math.max(0, Math.min(count - 1, index));
    slides[current].classList.remove("on");
    current = index;
    slides[current].classList.add("on");
    counter.textContent = current + 1 + " / " + count;
    prev.disabled = current === 0;
    next.disabled = current === count - 1;
    var text = slides[current].querySelector(".notes").textContent.trim();
    panel.textContent = "";
    var p = document.createElement("p");
    if (text) {
      p.textContent = text;
    } else {
      p.className = "none";
      p.textContent = "No notes for this slide.";
    }
    panel.appendChild(p);
    if (framed) parent.postMessage({ pictor: "slide", number: current + 1, count: count }, "*");
    place();
    try {
      var url = new URL(location.href);
      url.searchParams.set("slide", String(current + 1));
      history.replaceState(null, "", url);
    } catch (e) {
      /* a page without a writable address still presents */
    }
  }

  // In the viewer the shell puts its own editing fields over the slide's text.
  // It is told only where each editable string sits (data-edit, its place in
  // the slide; the box inside its padding), as fractions of this page's width
  // and height; the text itself the shell reads from the deck, never from
  // this page.
  function place() {
    if (!embedded) return;
    var w = window.innerWidth;
    var h = window.innerHeight;
    if (!w || !h) return;
    var fields = [];
    Array.prototype.forEach.call(slides[current].querySelectorAll("[data-edit]"), function (el) {
      var r = el.getBoundingClientRect();
      var style = getComputedStyle(el);
      var size = parseFloat(style.fontSize) || 16;
      var left = parseFloat(style.paddingLeft) || 0;
      var top = parseFloat(style.paddingTop) || 0;
      var across = left + (parseFloat(style.paddingRight) || 0);
      var down = top + (parseFloat(style.paddingBottom) || 0);
      fields.push({
        path: el.getAttribute("data-edit"),
        x: (r.left + left) / w, y: (r.top + top) / h,
        w: Math.max(0, r.width - across) / w, h: Math.max(0, r.height - down) / h,
        size: size / w, line: (parseFloat(style.lineHeight) || size * 1.2) / w,
        weight: parseInt(style.fontWeight, 10) || 400,
        align: style.textAlign
      });
    });
    parent.postMessage({ pictor: "fields", number: current + 1, fields: fields }, "*");
  }

  function toggleNotes() {
    var on = !deck.classList.contains("with-notes");
    deck.classList.toggle("with-notes", on);
    notesButton.setAttribute("aria-pressed", String(on));
  }

  function toggleFullscreen() {
    if (embedded) return;
    if (document.fullscreenElement) {
      document.exitFullscreen();
    } else {
      document.documentElement.requestFullscreen().catch(function () {});
    }
  }

  // In the viewer the shell draws the filmstrip and presents by putting this
  // frame full screen (?embed): the controls show only while presenting, and
  // the shell may name the slide to show. It is told which slide is showing.
  var params = new URLSearchParams(location.search);
  var framed = window.parent !== window;
  var embedded = framed && params.has("embed");
  var wanted = parseInt(params.get("slide") || "1", 10);
  deck.classList.add("js");
  deck.classList.toggle("embed", embedded);
  bar.hidden = false;
  show(isNaN(wanted) ? 0 : wanted - 1);
  if (!document.fullscreenEnabled || embedded) fullButton.hidden = true;
  if (embedded) {
    window.addEventListener("resize", place);
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(place);
  }
  window.addEventListener("message", function (event) {
    var m = event.data;
    if (event.source !== parent || !m || m.pictor !== "slides") return;
    if (typeof m.show === "number" && m.show >= 1) show(Math.floor(m.show) - 1);
    if (embedded && typeof m.present === "boolean") {
      deck.classList.toggle("full", m.present);
      wake();
    }
  });

  prev.addEventListener("click", function () { show(current - 1); });
  next.addEventListener("click", function () { show(current + 1); });
  notesButton.addEventListener("click", toggleNotes);
  fullButton.addEventListener("click", toggleFullscreen);
  document.addEventListener("fullscreenchange", function () {
    var on = !!document.fullscreenElement;
    deck.classList.toggle("full", on);
    fullButton.setAttribute("aria-pressed", String(on));
  });

  var keys = {
    ArrowRight: function () { show(current + 1); },
    ArrowDown: function () { show(current + 1); },
    PageDown: function () { show(current + 1); },
    " ": function () { show(current + 1); },
    ArrowLeft: function () { show(current - 1); },
    ArrowUp: function () { show(current - 1); },
    PageUp: function () { show(current - 1); },
    Home: function () { show(0); },
    End: function () { show(count - 1); },
    n: toggleNotes,
    f: toggleFullscreen
  };
  document.addEventListener("keydown", function (event) {
    var act = keys[event.key];
    if (!act || event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.target && event.target.tagName === "BUTTON" && (event.key === " " || event.key === "Enter")) return;
    event.preventDefault();
    act();
  });

  var touchX = null;
  document.addEventListener("touchstart", function (event) { touchX = event.changedTouches[0].clientX; }, { passive: true });
  document.addEventListener("touchend", function (event) {
    if (touchX === null) return;
    var dx = event.changedTouches[0].clientX - touchX;
    touchX = null;
    if (Math.abs(dx) > 50) show(current + (dx < 0 ? 1 : -1));
  }, { passive: true });

  var idle = 0;
  function wake() {
    bar.classList.remove("idle");
    clearTimeout(idle);
    idle = setTimeout(function () { bar.classList.add("idle"); }, 2500);
  }
  document.addEventListener("mousemove", wake);
  wake();
})();
