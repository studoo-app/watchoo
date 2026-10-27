// JavaScript de l'interface de Watchoo, chargé en defer sur toutes les pages : graphiques
// d'activité (Chart.js).
//
// Règles (CSP stricte : script-src 'self', sans eval) : aucune donnée interpolée dans ce
// fichier ; les données viennent d'un <script type="application/json" id="…"> lu par
// JSON.parse ; jamais d'innerHTML avec des données ; ne rien faire si l'élément visé est absent.
//
// Chaque graphique est un <canvas data-chart="…" data-source="…" [data-summary="…"]
// [data-refresh-url="…"]> :
//   - data-chart : forme du graphique,
//       « daily »          : barres des commits par jour, données [{date: "2026-10-27", count: 3}, …]
//                            (détail d'un dépôt, tableau de bord) ;
//       « weekly-stacked » : barres empilées des commits par semaine, une série par contributeur,
//                            données {weeks: ["2026-10-26", …], series: [{name, counts: [n, …],
//                            total, other}, …]} (page des contributeurs) ;
//   - data-source : id du <script type="application/json"> qui porte les données (calculées côté
//     Go dans le fuseau d'affichage, internal/web/charts.go) ;
//   - data-summary : id du résumé textuel, remplacé lors d'un rafraîchissement ;
//   - data-table : id de la table des données (vue sans graphique), remplacée lors d'un
//     rafraîchissement par le nœud de la page relue (importNode, jamais innerHTML) ;
//   - data-refresh-url : page (même origine) relue à la fin d'une synchro.
// Chart.js (window.Chart) n'est chargé que sur les pages qui ont un graphique, en defer après ce
// fichier : le dessin attend donc DOMContentLoaded, qui suit l'exécution de tous les scripts defer.
//
// Mises à jour :
//   - changement de thème (événement « watchoo:theme » de theme.js, ou préférence du système) :
//     couleurs relues dans les variables CSS de DaisyUI, palette claire ou sombre ;
//   - fin d'une synchro (événement HTMX « sync-finished » sur <body>) : HTMX retire les <script>
//     des fragments qu'il échange (allowScriptTags:false), les graphiques ne sont donc pas des
//     fragments ; on relit la page complète (data-refresh-url), on en extrait les données avec
//     DOMParser (qui n'exécute aucun script) et on redessine.
(function () {
  "use strict";

  var charts = {}; // id du canvas → instance Chart.js

  // Palette catégorielle (identité d'un contributeur) : huit teintes dans un ordre fixe, validées
  // pour les daltonismes courants, avec une variante par thème (la même teinte, ajustée pour le
  // fond sombre). La neuvième série et les suivantes sont regroupées côté Go (« Autres
  // contributeurs », couleur neutre) : aucune couleur n'est jamais générée.
  var CATEGORICAL = {
    light: ["#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#6250d6", "#e34948"],
    dark: ["#3987e5", "#d95926", "#199e70", "#c98500", "#d55181", "#008300", "#9085e9", "#e66767"]
  };
  var OTHER = { light: "#a9a8a1", dark: "#6b6a64" };

  // cssColor lit une variable CSS de DaisyUI (ex. --color-primary), avec une valeur de repli.
  function cssColor(name, fallback) {
    var v = window.getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }

  // toRGB convertit une couleur CSS quelconque (DaisyUI 5 utilise oklch, que Chart.js ne sait
  // pas analyser) en [r, g, b] : le navigateur la peint sur un canvas de 1 px et on relit le pixel.
  var probe = null;
  function toRGB(color) {
    if (!probe) {
      probe = document.createElement("canvas");
      probe.width = probe.height = 1;
    }
    var ctx = probe.getContext("2d", { willReadFrequently: true });
    if (!ctx) {
      return [99, 102, 241];
    }
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = "#6366f1";
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, 1, 1);
    var px = ctx.getImageData(0, 0, 1, 1).data;
    return [px[0], px[1], px[2]];
  }

  function rgba(color, alpha) {
    var c = toRGB(color);
    return "rgba(" + c[0] + ", " + c[1] + ", " + c[2] + ", " + alpha + ")";
  }

  // dark indique si le thème affiché est sombre : luminance du fond de DaisyUI (--color-base-100),
  // qui reflète à la fois le choix enregistré (data-theme) et la préférence du système.
  function dark() {
    var c = toRGB(cssColor("--color-base-100", "#ffffff"));
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2] < 128;
  }

  function palette() {
    var text = cssColor("--color-base-content", "#1f2937");
    var primary = cssColor("--color-primary", "#4f46e5");
    var mode = dark() ? "dark" : "light";
    return {
      bar: rgba(primary, 0.85),
      hover: rgba(primary, 1),
      text: rgba(text, 0.7),
      grid: rgba(text, 0.12),
      surface: rgba(cssColor("--color-base-100", "#ffffff"), 1),
      series: CATEGORICAL[mode],
      other: OTHER[mode]
    };
  }

  var DAY_RE = /^\d{4}-\d{2}-\d{2}$/;

  function count(v) {
    var n = Number(v);
    return Number.isFinite(n) && n > 0 ? Math.floor(n) : 0;
  }

  // parseDaily lit et valide une série par jour : tableau d'objets {date: "AAAA-MM-JJ", count}.
  function parseDaily(raw) {
    if (!Array.isArray(raw)) {
      return null;
    }
    var out = [];
    for (var i = 0; i < raw.length; i++) {
      var d = raw[i];
      if (!d || typeof d.date !== "string" || !DAY_RE.test(d.date)) {
        return null;
      }
      out.push({ date: d.date, count: count(d.count) });
    }
    return out;
  }

  // parseWeekly lit et valide les séries par semaine : {weeks: [...], series: [{name, counts}]}.
  function parseWeekly(raw) {
    if (!raw || !Array.isArray(raw.weeks) || !Array.isArray(raw.series)) {
      return null;
    }
    for (var i = 0; i < raw.weeks.length; i++) {
      if (typeof raw.weeks[i] !== "string" || !DAY_RE.test(raw.weeks[i])) {
        return null;
      }
    }
    var series = [];
    for (var j = 0; j < raw.series.length; j++) {
      var s = raw.series[j];
      if (!s || typeof s.name !== "string" || !Array.isArray(s.counts) || s.counts.length !== raw.weeks.length) {
        return null;
      }
      series.push({ name: s.name, counts: s.counts.map(count), other: s.other === true });
    }
    return { weeks: raw.weeks.slice(), series: series };
  }

  var PARSERS = { "daily": parseDaily, "weekly-stacked": parseWeekly };

  // parse lit le JSON text pour la forme kind ; null s'il est invalide.
  function parse(kind, text) {
    var parser = PARSERS[kind];
    if (!parser) {
      return null;
    }
    try {
      return parser(JSON.parse(text || "null"));
    } catch (e) {
      return null;
    }
  }

  // Dates : les séries donnent des jours calendaires (fuseau d'affichage) ; on les formate sans
  // conversion de fuseau, en les traitant comme des dates UTC.
  var longDate = new Intl.DateTimeFormat("fr-FR", {
    weekday: "long", day: "numeric", month: "long", year: "numeric", timeZone: "UTC"
  });
  var dayMonth = new Intl.DateTimeFormat("fr-FR", { day: "numeric", month: "long", timeZone: "UTC" });

  function dayDate(s) {
    var p = s.split("-");
    return new Date(Date.UTC(Number(p[0]), Number(p[1]) - 1, Number(p[2])));
  }

  function shortLabel(s) {
    var p = s.split("-");
    return p[2] + "/" + p[1];
  }

  function weekTitle(s) {
    var start = dayDate(s);
    var end = new Date(start.getTime() + 6 * 24 * 3600 * 1000);
    return "Semaine du " + dayMonth.format(start) + " au " + longDate.format(end).replace(/^\S+ /, "");
  }

  function commits(n) {
    return n + (n > 1 ? " commits" : " commit");
  }

  function reducedMotion() {
    return window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  function axes(colors, stacked) {
    return {
      x: {
        stacked: stacked,
        grid: { display: false },
        ticks: { color: colors.text, maxRotation: 0, autoSkip: true, maxTicksLimit: 12 }
      },
      y: {
        stacked: stacked,
        beginAtZero: true,
        grid: { color: colors.grid },
        ticks: { color: colors.text, precision: 0 }
      }
    };
  }

  function dailyConfig(series, colors) {
    return {
      type: "bar",
      data: {
        labels: series.map(function (d) { return shortLabel(d.date); }),
        datasets: [{
          label: "Commits",
          data: series.map(function (d) { return d.count; }),
          backgroundColor: colors.bar,
          hoverBackgroundColor: colors.hover,
          borderRadius: 2,
          maxBarThickness: 14
        }]
      },
      options: {
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
        scales: axes(colors, false)
      }
    };
  }

  function weeklyConfig(data, colors) {
    var slot = 0;
    var datasets = data.series.map(function (s) {
      var color = s.other ? colors.other : colors.series[slot++ % colors.series.length];
      return {
        label: s.name,
        data: s.counts,
        backgroundColor: color,
        hoverBackgroundColor: color,
        // Liseré de la couleur du fond entre deux segments empilés.
        borderColor: colors.surface,
        borderWidth: { top: 2 },
        borderSkipped: "bottom",
        maxBarThickness: 32
      };
    });
    return {
      type: "bar",
      data: { labels: data.weeks.map(shortLabel), datasets: datasets },
      options: {
        interaction: { mode: "index", intersect: false },
        plugins: {
          legend: {
            display: datasets.length > 1,
            position: "bottom",
            labels: { color: colors.text, boxWidth: 12, boxHeight: 12 }
          },
          tooltip: {
            filter: function (item) { return item.parsed.y > 0; },
            callbacks: {
              title: function (items) {
                return items.length ? weekTitle(data.weeks[items[0].dataIndex]) : "";
              },
              label: function (item) { return item.dataset.label + " : " + commits(item.parsed.y); },
              footer: function (items) {
                var total = 0;
                for (var i = 0; i < items.length; i++) {
                  total += items[i].parsed.y;
                }
                return items.length > 1 ? "Total : " + commits(total) : "";
              }
            }
          }
        },
        scales: axes(colors, true)
      }
    };
  }

  var CONFIGS = { "daily": dailyConfig, "weekly-stacked": weeklyConfig };

  // draw (re)dessine le graphique du canvas à partir de ses données.
  function draw(canvas) {
    var kind = canvas.getAttribute("data-chart");
    var source = document.getElementById(canvas.getAttribute("data-source") || "");
    if (!source || !CONFIGS[kind] || typeof window.Chart !== "function") {
      return;
    }
    var data = parse(kind, source.textContent);
    if (!data) {
      return;
    }
    var config = CONFIGS[kind](data, palette());
    config.options.responsive = true;
    config.options.maintainAspectRatio = false;
    config.options.animation = reducedMotion() ? false : { duration: 300 };
    if (charts[canvas.id]) {
      charts[canvas.id].destroy();
    }
    charts[canvas.id] = new window.Chart(canvas, config);
  }

  function canvases() {
    return Array.prototype.slice.call(document.querySelectorAll("canvas[data-chart][data-source][id]"));
  }

  function renderAll() {
    canvases().forEach(draw);
  }

  // copyText remplace le texte de l'élément id de la page par celui de doc (s'ils existent).
  function copyText(doc, id) {
    if (!id) {
      return;
    }
    var fresh = doc.getElementById(id);
    var current = document.getElementById(id);
    if (fresh && current) {
      current.textContent = fresh.textContent;
    }
  }

  // copyNode remplace l'élément id de la page par une copie de celui de doc (s'ils existent). Le
  // document de DOMParser est inerte ; le nœud copié ne contient que du balisage de table.
  function copyNode(doc, id) {
    if (!id) {
      return;
    }
    var fresh = doc.getElementById(id);
    var current = document.getElementById(id);
    if (fresh && current && fresh.tagName === current.tagName && !fresh.querySelector("script")) {
      current.replaceWith(document.importNode(fresh, true));
    }
  }

  // refresh relit la page de chaque graphique (une fois par adresse) et remplace ses données.
  function refresh() {
    var byURL = {};
    canvases().forEach(function (canvas) {
      var url = canvas.getAttribute("data-refresh-url");
      if (!url || url.charAt(0) !== "/" || url.charAt(1) === "/" || url.charAt(1) === "\\") {
        return;
      }
      (byURL[url] = byURL[url] || []).push(canvas);
    });
    Object.keys(byURL).forEach(function (url) {
      window.fetch(url, { credentials: "same-origin", headers: { "Accept": "text/html" } })
        .then(function (res) { return res.ok ? res.text() : Promise.reject(res.status); })
        .then(function (html) {
          var doc = new DOMParser().parseFromString(html, "text/html");
          byURL[url].forEach(function (canvas) {
            var id = canvas.getAttribute("data-source");
            var source = document.getElementById(id);
            var fresh = doc.getElementById(id);
            if (!source || !fresh || !parse(canvas.getAttribute("data-chart"), fresh.textContent)) {
              return;
            }
            source.textContent = fresh.textContent;
            copyText(doc, canvas.getAttribute("data-summary"));
            copyNode(doc, canvas.getAttribute("data-table"));
            var label = doc.getElementById(canvas.id);
            if (label && label.getAttribute("aria-label")) {
              canvas.setAttribute("aria-label", label.getAttribute("aria-label"));
            }
            draw(canvas);
          });
        })
        .catch(function () {
          // page retirée, serveur arrêté… : on garde les graphiques affichés
        });
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    if (!canvases().length) {
      return;
    }
    renderAll();
    document.addEventListener("watchoo:theme", renderAll);
    if (window.matchMedia) {
      var mq = window.matchMedia("(prefers-color-scheme: dark)");
      if (mq.addEventListener) {
        mq.addEventListener("change", renderAll);
      }
    }
    document.body.addEventListener("sync-finished", refresh);
  });
})();
