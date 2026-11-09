// bowel's website: language, screenshots that follow it and the color
// scheme, the skin gallery, copy buttons, and a little entrance motion.
(function () {
  var root = document.documentElement;
  var dark = window.matchMedia("(prefers-color-scheme: dark)");
  var lang = function () { return root.classList.contains("en") ? "en" : "zh"; };

  // Every <img data-shot="name"> shows shots/name-<lang>[-dark].webp.
  // data-dark="1" keeps it dark whatever the page's scheme.
  function shots() {
    document.querySelectorAll("img[data-shot]").forEach(function (img) {
      var d = img.dataset.dark === "1" || dark.matches;
      var src = "shots/" + img.dataset.shot + "-" + lang() + (d ? "-dark" : "") + ".webp";
      if (img.getAttribute("src") !== src) img.setAttribute("src", src);
    });
    skin(current, true);
  }

  function title() {
    document.title = lang() === "en" ? "bowel · A cross-platform mihomo client" : "bowel · 跨平台的 mihomo 客户端";
  }
  function setLang(l) {
    root.className = l;
    root.lang = l === "en" ? "en" : "zh-CN";
    title();
    try { localStorage.setItem("bowel-lang", l); } catch (e) {}
    shots();
  }
  document.querySelector(".lang").addEventListener("click", function () {
    setLang(lang() === "en" ? "zh" : "en");
  });
  dark.addEventListener("change", shots);

  // Skins.
  var current = "paper";
  var skinImg = document.getElementById("skin-img");
  var tabs = document.querySelectorAll(".skin-tabs button");
  function skin(id, quiet) {
    current = id;
    tabs.forEach(function (b) { b.setAttribute("aria-selected", String(b.dataset.skin === id)); });
    document.querySelectorAll("[data-note]").forEach(function (n) { n.hidden = n.dataset.note !== id; });
    var src = "shots/skin-" + id + "-" + lang() + ".webp";
    if (skinImg.getAttribute("src") === src) return;
    if (quiet) { skinImg.setAttribute("src", src); return; }
    var next = new Image();
    next.src = src;
    skinImg.classList.add("fading");
    next.decode().catch(function () {}).then(function () {
      skinImg.setAttribute("src", src);
      skinImg.classList.remove("fading");
    });
  }
  tabs.forEach(function (b) {
    b.addEventListener("click", function () { skin(b.dataset.skin); });
    // Warm the cache so switching is instant.
    b.addEventListener("pointerenter", function () { new Image().src = "shots/skin-" + b.dataset.skin + "-" + lang() + ".webp"; }, { once: true });
  });

  // Copy buttons.
  document.querySelectorAll(".copy-btn").forEach(function (b) {
    b.addEventListener("click", function () {
      navigator.clipboard.writeText(b.dataset.copy).then(function () {
        b.classList.add("done");
        setTimeout(function () { b.classList.remove("done"); }, 1400);
      });
    });
  });

  // The nav gets a hairline once the page scrolls.
  var nav = document.querySelector(".nav");
  var onScroll = function () { nav.classList.toggle("scrolled", window.scrollY > 8); };
  window.addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  // Sections rise in as they come into view.
  if ("IntersectionObserver" in window) {
    var io = new IntersectionObserver(function (es) {
      es.forEach(function (e) { if (e.isIntersecting) { e.target.classList.add("in"); io.unobserve(e.target); } });
    }, { rootMargin: "0px 0px -8% 0px" });
    document.querySelectorAll(".feature, .agents > *, .cell, .skins > *, .download > *").forEach(function (el) {
      el.classList.add("reveal");
      io.observe(el);
    });
  }

  title();
  shots();
})();
