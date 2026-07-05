package main

import (
	"fmt"

	webview "github.com/webview/webview_go"
)

// @sk-task kvn-desktop#T3.2: inject restart button via JS (AC-009, AC-010, AC-011)
func injectRestartButton(w webview.WebView, svc *ServiceManager) {
	// #nosec G104
	w.Bind("restartService", func() {
		if err := svc.Restart(); err != nil {
			w.Eval(fmt.Sprintf(
				`console.error('restart failed: %s')`,
				escapeJS(err.Error()),
			))
			return
		}
		w.Eval(`setTimeout(() => location.reload(), 1000)`)
	})

	w.Init(restartButtonJS)
}

const restartButtonJS = `
(function() {
  if (window.location.hostname !== '127.0.0.1' && window.location.hostname !== 'localhost') return;
  function kvnAddRestartBtn() {
    if (document.getElementById('kvn-restart-btn')) return;
    var btn = document.createElement('button');
    btn.id = 'kvn-restart-btn';
    btn.innerText = 'Restart Service';
    btn.style.cssText = 'position:fixed;top:8px;right:8px;z-index:9999;' +
      'background:#1a5a9e;color:#fff;border:none;padding:5px 12px;' +
      'border-radius:4px;font-size:12px;cursor:pointer;font-weight:600;' +
      'font-family:"Segoe UI",system-ui,sans-serif;' +
      'box-shadow:0 2px 6px rgba(0,0,0,0.4);';
    btn.onmouseenter = function() { btn.style.background = '#1e6ab8'; };
    btn.onmouseleave = function() { btn.style.background = '#1a5a9e'; };
    btn.onclick = function() {
      btn.disabled = true;
      btn.innerText = 'Restarting...';
      window.restartService();
    };
    document.body.appendChild(btn);
  }
  if (document.readyState !== 'loading') {
    kvnAddRestartBtn();
  } else {
    document.addEventListener('DOMContentLoaded', kvnAddRestartBtn);
  }
})();
`
