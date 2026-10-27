// JavaScript de l'interface de Watchoo, chargé en defer sur toutes les pages.
// PROPRIÉTAIRE : agent repo (graphique d'activité de la page de détail d'un dépôt).
//
// Règles (CSP stricte : script-src 'self', sans eval) : aucune donnée interpolée dans ce
// fichier ; les données viennent d'un <script type="application/json" id="…"> lu par
// JSON.parse ; jamais d'innerHTML avec des données ; ne rien faire si l'élément visé est absent.
//
// Graphique d'activité : <canvas id="activity-chart"> et ses données <script
// type="application/json" id="activity-data"> ([{date: "2026-10-27", count: 3}, …], calculées
// côté Go dans le fuseau d'affichage). Chart.js (window.Chart) n'est chargé que sur cette page,
// en defer après ce fichier : le dessin attend donc DOMContentLoaded, qui suit l'exécution de
// tous les scripts defer.
//
// Mises à jour :
//   - changement de thème (événement « watchoo:theme » de theme.js, ou préférence du système) :
//     couleurs relues dans les variables CSS de DaisyUI ;
//   - fin d'une synchro (événement HTMX « sync-finished » sur <body>) : HTMX retire les <script>
//     des fragments qu'il échange (allowScriptTags:false), le graphique n'est donc pas un
//     fragment ; on relit la page complète (data-refresh-url, même origine), on en extrait
//     #activity-data avec DOMParser (qui n'exécute aucun script) et on remplace les données.
(function () {
  "use strict";

  var chart = null;

  // cssColor lit une variable CSS de DaisyUI (ex. --color-primary), avec une valeur de repli.
  function cssColor(name, fallback) {
    var v = window.getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }

  // rgba convertit une couleur CSS quelconque (DaisyUI 5 utilise oklch, que Chart.js ne sait
  // pas analyser) en « rgba(r, g, b, alpha) » : le navigateur la peint sur un canvas de 1 px et
  // on relit le pixel.
  var probe = null;
  function rgba(color, alpha) {
    if (!probe) {
      probe = document.createElement("canvas");
      probe.width = probe.height = 1;
    }
    var ctx = probe.getContext("2d", { willReadFrequently: true });
    if (!ctx) {
      return "rgba(99, 102, 241, " + alpha + ")";
    }
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = "#6366f1";
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, 1, 1);
    var px = ctx.getImageData(0, 0, 1, 1).data;
    return "rgba(" + px[0] + ", " + px[1] + ", " + px[2] + ", " + alpha + ")";
  }

  function palette() {
    var text = cssColor("--color-base-content", "#1f2937");
    var primary = cssColor("--color-primary", "#4f46e5");
    return {
      bar: rgba(primary, 0.85),
      hover: rgba(primary, 1),
      text: rgba(text, 0.7),
      grid: rgba(text, 0.12)
    };
  }

  // parseData lit et valide la série : tableau d'objets {date: "AAAA-MM-JJ", count: entier >= 0}.
  function parseData(text) {
    var raw;
    try {
      raw = JSON.parse(text || "[]");
    } catch (e) {
      return null;
    }
    if (!Array.isArray(raw)) {
      return null;
    }
    var out = [];
    for (var i = 0; i < raw.length; i++) {
      var d = raw[i];
      if (!d || typeof d.date !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(d.date)) {
        return null;
      }
      var n = Number(d.count);
      out.push({ date: d.date, count: Number.isFinite(n) && n > 0 ? Math.floor(n) : 0 });
    }
    return out;
  }

  // Dates : la série donne des jours calendaires (fuseau d'affichage) ; on les formate sans
  // conversion de fuseau, en les traitant comme des dates UTC.
  var longDate = new Intl.DateTimeFormat("fr-FR", {
    weekday: "long", day: "numeric", month: "long", year: "numeric", timeZone: "UTC"
  });

  function dayDate(s) {
    var p = s.split("-");
    return new Date(Date.UTC(Number(p[0]), Number(p[1]) - 1, Number(p[2])));
  }

  function shortLabel(s) {
    var p = s.split("-");
    return p[2] + "/" + p[1];
  }

  function commits(n) {
    return n + (n > 1 ? " commits" : " commit");
  }

  function reducedMotion() {
    return window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  function draw(canvas, series) {
    var colors = palette();
    var labels = series.map(function (d) { return shortLabel(d.date); });
    var values = series.map(function (d) { return d.count; });
    if (chart) {
      chart.destroy();
    }
    chart = new window.Chart(canvas, {
      type: "bar",
      data: {
        labels: labels,
        datasets: [{
          label: "Commits",
          data: values,
          backgroundColor: colors.bar,
          hoverBackgroundColor: colors.hover,
          borderRadius: 2,
          maxBarThickness: 14
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: reducedMotion() ? false : { duration: 300 },
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              title: function (items) {
                return items.length ? longDate.format(dayDate(series[items[0].dataIndex].date)) : "";
              },
              label: function (item) { return commits(item.parsed.y); }
            }
          }
        },
        scales: {
          x: {
            grid: { display: false },
            ticks: { color: colors.text, maxRotation: 0, autoSkip: true, maxTicksLimit: 10 }
          },
          y: {
            beginAtZero: true,
            grid: { color: colors.grid },
            ticks: { color: colors.text, precision: 0 }
          }
        }
      }
    });
  }

  function render() {
    var canvas = document.getElementById("activity-chart");
    var source = document.getElementById("activity-data");
    if (!canvas || !source || typeof window.Chart !== "function") {
      return;
    }
    var series = parseData(source.textContent);
    if (series) {
      draw(canvas, series);
    }
  }

  // refresh relit la page du dépôt et remplace les données du graphique.
  function refresh() {
    var canvas = document.getElementById("activity-chart");
    var source = document.getElementById("activity-data");
    var url = canvas && canvas.getAttribute("data-refresh-url");
    if (!canvas || !source || !url || url.charAt(0) !== "/" || url.charAt(1) === "/") {
      return;
    }
    window.fetch(url, { credentials: "same-origin", headers: { "Accept": "text/html" } })
      .then(function (res) { return res.ok ? res.text() : Promise.reject(res.status); })
      .then(function (html) {
        var doc = new DOMParser().parseFromString(html, "text/html");
        var fresh = doc.getElementById("activity-data");
        var summary = doc.getElementById("activity-summary");
        if (!fresh || !parseData(fresh.textContent)) {
          return;
        }
        source.textContent = fresh.textContent;
        var current = document.getElementById("activity-summary");
        if (summary && current) {
          current.textContent = summary.textContent;
        }
        var label = doc.getElementById("activity-chart");
        if (label && label.getAttribute("aria-label")) {
          canvas.setAttribute("aria-label", label.getAttribute("aria-label"));
        }
        render();
      })
      .catch(function () {
        // dépôt retiré, serveur arrêté… : on garde le graphique affiché
      });
  }

  document.addEventListener("DOMContentLoaded", function () {
    if (!document.getElementById("activity-chart")) {
      return;
    }
    render();
    document.addEventListener("watchoo:theme", render);
    if (window.matchMedia) {
      var mq = window.matchMedia("(prefers-color-scheme: dark)");
      if (mq.addEventListener) {
        mq.addEventListener("change", render);
      }
    }
    document.body.addEventListener("sync-finished", refresh);
  });
})();
