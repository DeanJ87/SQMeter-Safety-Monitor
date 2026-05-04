(function () {
  var valid = ["overview", "configuration", "safety-monitor", "observing-conditions", "diagnostics"];
  var shell = document.querySelector(".bridge-shell");
  var initialPage = shell ? shell.getAttribute("data-initial-page") : "overview";

  function normalizedHash() {
    var h = window.location.hash.replace(/^#/, "");
    if (valid.indexOf(h) === -1) {
      return valid.indexOf(initialPage) === -1 ? "overview" : initialPage;
    }
    return h;
  }

  function showPage(page) {
    if (valid.indexOf(page) === -1) page = "overview";
    document.querySelectorAll(".page").forEach(function (el) {
      el.classList.toggle("active", el.getAttribute("data-page") === page);
    });
    document.querySelectorAll(".nav-item").forEach(function (el) {
      el.classList.toggle("active", el.getAttribute("data-page") === page);
    });
  }

  window.addEventListener("hashchange", function () {
    showPage(normalizedHash());
  });
  document.querySelectorAll(".nav-item").forEach(function (el) {
    el.addEventListener("click", function () {
      var page = el.getAttribute("data-page");
      if (window.location.hash === "#" + page) showPage(page);
    });
  });
  if (!window.location.hash && initialPage !== "overview") {
    showPage(initialPage);
  } else {
    showPage(normalizedHash());
  }

  document.querySelectorAll(".filter-chip").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var filter = btn.getAttribute("data-filter");
      document.querySelectorAll(".filter-chip").forEach(function (b) { b.classList.toggle("active", b === btn); });
      document.querySelectorAll("#oc-table tbody tr").forEach(function (row) {
        var status = row.getAttribute("data-status");
        var show = filter === "all" || status === filter || (filter === "missing" && status === "unsupported");
        row.classList.toggle("hide-when-filtered", !show);
      });
    });
  });

  document.querySelectorAll(".copy-endpoint").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var value = btn.getAttribute("data-copy");
      var done = function () {
        var old = btn.textContent;
        btn.textContent = "copied";
        window.setTimeout(function () { btn.textContent = old; }, 900);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(value).then(done, done);
      } else {
        done();
      }
    });
  });

  var testButton = document.getElementById("test-sqmeter");
  if (testButton) {
    testButton.addEventListener("click", function () {
      var result = document.getElementById("test-result");
      var url = document.getElementById("sqmeter-url").value.trim();
      result.textContent = "Testing...";
      fetch("/api/test-sqmeter", {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify({url: url})
      }).then(function (r) {
        return r.json();
      }).then(function (d) {
        result.textContent = (d.ok ? "OK: " : "ERROR: ") + d.message;
        result.className = "hint mono " + (d.ok ? "green" : "red");
      }).catch(function (err) {
        result.textContent = "ERROR: " + err;
        result.className = "hint mono red";
      });
    });
  }

  document.querySelectorAll("[data-service]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      fetch("/api/service/" + btn.getAttribute("data-service"), {method: "POST"});
    });
  });
}());
