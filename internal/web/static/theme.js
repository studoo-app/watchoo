// Thème clair / sombre de Watchoo (socle). Chargé dans <head>, sans defer, pour appliquer le
// thème choisi avant le premier affichage. Sans choix enregistré, DaisyUI suit
// prefers-color-scheme (thème dark « prefersdark »). Le choix est gardé dans localStorage
// (clé watchoo-theme : « light » ou « dark ») ; aucune autre donnée n'y est écrite.
(function () {
  "use strict";
  var KEY = "watchoo-theme";
  var root = document.documentElement;

  function stored() {
    try {
      var v = window.localStorage.getItem(KEY);
      return v === "light" || v === "dark" ? v : null;
    } catch (e) {
      return null;
    }
  }

  function current() {
    var v = stored();
    if (v) {
      return v;
    }
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }

  function apply(theme) {
    root.setAttribute("data-theme", theme);
  }

  var initial = stored();
  if (initial) {
    apply(initial);
  }

  document.addEventListener("DOMContentLoaded", function () {
    var button = document.getElementById("theme-toggle");
    if (!button) {
      return;
    }
    button.addEventListener("click", function () {
      var next = current() === "dark" ? "light" : "dark";
      apply(next);
      try {
        window.localStorage.setItem(KEY, next);
      } catch (e) {
        // stockage indisponible (navigation privée) : le thème vaut pour cette page seulement
      }
      document.dispatchEvent(new CustomEvent("watchoo:theme", { detail: { theme: next } }));
    });
  });
})();
