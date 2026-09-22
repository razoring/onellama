package ui

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// modelsSearchHandler acts as a proxy/parser for ollama.com/search.
func (s *Server) modelsSearchHandler(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query().Get("q")
	cap := r.URL.Query().Get("c")
	o := r.URL.Query().Get("o")

	target, err := url.Parse("https://ollama.com/search")
	if err != nil {
		http.Error(w, "invalid target url", http.StatusInternalServerError)
		return nil
	}
	query := target.Query()
	if q != "" {
		query.Set("q", q)
	}
	if cap != "" {
		query.Set("c", cap)
	}
	if o != "" {
		query.Set("o", o)
	}
	target.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("upstream returned %d", resp.StatusCode), resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream body", http.StatusInternalServerError)
		return nil
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"html":%q}`, string(body))
	return nil
}

// modelsWebviewHandler proxies ollama.com pages (search, library detail) and strips X-Frame-Options & navbar.
func (s *Server) modelsWebviewHandler(w http.ResponseWriter, r *http.Request) error {
	path := r.URL.Query().Get("path")
	theme := r.URL.Query().Get("theme")

	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "search"
	}

	// Prevent open proxy abuse (only allow paths on ollama.com, forbid scheme/domain injection)
	if strings.Contains(path, "://") || strings.HasPrefix(path, "//") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return nil
	}

	targetURLStr := fmt.Sprintf("https://ollama.com/%s", path)
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}

	targetQuery := targetURL.Query()
	for k, vv := range r.URL.Query() {
		if k == "path" || k == "theme" {
			continue
		}
		for _, v := range vv {
			targetQuery.Add(k, v)
		}
	}
	targetURL.RawQuery = targetQuery.Encode()

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	// Copy HX- headers from the client
	for k, vv := range r.Header {
		if strings.HasPrefix(strings.ToUpper(k), "HX-") {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return nil
	}

	// Strip restrictive headers and copy rest
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "x-frame-options" || lk == "content-security-policy" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	// Remove the DENY header set by the ui.go handler wrapper so this can be iframed
	w.Header().Del("X-Frame-Options")

	htmlStr := string(body)
	contentType := resp.Header.Get("Content-Type")

	// Inject base tag, navbar hiding CSS, theme styles, and link interceptor for HTML responses
	if strings.Contains(contentType, "html") || strings.Contains(htmlStr, "<head>") {
		var themeCSS string
		if theme == "dark" || theme == "" {
			themeCSS = `<style id="onellama-theme">
/* ---------- onellama dark theme for ollama.com (webview) ---------- */
/* Canonical override — keep in sync with server/models_registry.go and search.html */
:root { color-scheme: dark; }

/* Canvas */
html, body, main { background-color:#171717 !important; color-scheme:dark !important; }
body, main { color:#f5f5f5 !important; }

/* Header shell */
header, header.sticky, header.bg-white, .bg-white { background-color:#171717 !important; border-color:#333 !important; }
header nav a, header span { color:#e5e5e5 !important; }

/* Primary / secondary text ramp */
.text-black, .text-neutral-900, h1, h2, h3, h4, h5, h6, strong, b, .prose, .prose * { color:#f5f5f5 !important; }
.text-neutral-800 { color:#d4d4d4 !important; }
.text-neutral-700 { color:#d4d4d4 !important; }
.text-neutral-600, .text-neutral-500, .text-neutral-400, .text-gray-500, .text-gray-600 { color:#a3a3a3 !important; }
.text-gray-400 { color:#9ca3af !important; }
.text-white { color:#fff !important; }

/* Semantic accents — site blue/indigo/green kept but lightened for dark bg */
.text-blue-600 { color:#93c5fd !important; }
.text-indigo-600 { color:#c7d2fe !important; }
.text-green-700 { color:#6ee7b7 !important; }
.bg-cyan-50 { background-color:rgba(34,211,238,0.15) !important; }
.text-cyan-500 { color:#67e8f9 !important; }
.bg-indigo-50 { background-color:rgba(99,102,241,0.15) !important; }
.bg-\[#ddf4ff\] { background-color:rgba(59,130,246,0.15) !important; }

/* Surfaces */
.bg-neutral-50 { background-color:#1f1f1f !important; }
[class*="bg-white"] { background-color:#171717 !important; }
.bg-black\/5, div[class*="bg-black/5"] { background-color:rgba(255,255,255,0.04) !important; }
.bg-neutral-800 { background-color:#404040 !important; color:#fff !important; }
a.bg-neutral-800:hover, a.bg-neutral-800:focus, .focus\:bg-black { background-color:#525252 !important; }

/* Filter chips — unselected stays transparent with neutral ink; selected lifts */
[class*="bg-black"] { background-color:rgba(255,255,255,0.04) !important; color:#b3b3b3 !important; }
.peer:checked ~ .peer-checked\:bg-neutral-100 { background-color:rgba(255,255,255,0.10) !important; }
.min-md\:hover\:bg-neutral-100:hover { background-color:rgba(255,255,255,0.06) !important; }
[class*="bg-neutral-800"], a[class*="bg-neutral-800"] { background-color:rgba(255,255,255,0.08) !important; color:#f5f5f5 !important; border-color:#333 !important; }
a[class*="bg-neutral-800"]:hover, a[class*="bg-neutral-800"]:focus { background-color:rgba(255,255,255,0.12) !important; }

/* Code / prose */
pre, code, .prose-pre pre, .prose-code code, code\:bg-gray-200 { background-color:#1a1a1a !important; color:#d4d4d4 !important; }
md-pre, md-code, md-bold, md-italic { color:#d4d4d4 !important; }

/* Detail page lists / bullets surfaced as plain spans */
.prose ul, .prose ol, .prose li, ul, ol, li, [class*="markdown"], .md-text { color:#e5e5e5 !important; }
li::marker { color:#d4d4d4 !important; }

/* Controls */
input, select, textarea { background-color:#171717 !important; border-color:#333 !important; color:#e5e5e5 !important; }
input[type="text"], input[type="search"] { background-color:transparent !important; }
input::placeholder, textarea::placeholder { color:#6b7280 !important; }
select:hover, option { background-color:#1c1c1c !important; color:#e5e5e5 !important; }
.border-neutral-100, .border-neutral-200, .border-gray-200 { border-color:#333 !important; }

/* Scrollbar */
::-webkit-scrollbar { width:10px; height:10px; }
::-webkit-scrollbar-thumb { background:#333; border-radius:8px; }
</style>`
		} else {
			themeCSS = `<style id="onellama-theme">
				html { color-scheme: light !important; background: #ffffff !important; }
			</style>`
		}

		injectedCode := fmt.Sprintf(`
			<base href="https://ollama.com/" />
			<style id="onellama-hide-nav">
				header nav > a,
				header nav > div:not(.flex-grow),
				header .lg\:hidden { display: none !important; }
				header nav { justify-content: center !important; padding: 12px 24px !important; }
				footer a[href="/blog"],
				footer a[href^="mailto:support"] { display: none !important; }
				footer .justify-between { justify-content: center !important; }
				body { margin-top: 0 !important; padding-top: 0 !important; }
			</style>
			%s
			<script>
				(function() {
					function getCurrentTheme() {
						return new URLSearchParams(window.location.search).get('theme') || 'dark';
					}

					function toProxyUrl(urlStr) {
						if (!urlStr) return '';
						try {
							var u = new URL(urlStr, window.location.href);
							if (u.hostname === 'ollama.com' || u.hostname === window.location.hostname) {
								var rawPath = u.pathname.replace(/^\/+/, '');
								if (!rawPath) return '';
								if (rawPath.startsWith('api/v1/models/webview')) return '';

								var fullPath = rawPath + u.search + u.hash;
								return window.location.origin + '/api/v1/models/webview?path=' + encodeURIComponent(fullPath) + '&theme=' + encodeURIComponent(getCurrentTheme());
							}
						} catch(e) {}
						return '';
					}

					document.addEventListener('click', function(e) {
						var a = e.target.closest('a');
						if (a) {
							var rawHref = a.getAttribute('href') || a.href;
							var targetProxy = toProxyUrl(rawHref);
							if (targetProxy) {
								e.preventDefault();
								e.stopPropagation();
								window.location.href = targetProxy;
							}
						}
					}, true);

					document.addEventListener('submit', function(e) {
						var form = e.target;
						var action = form.getAttribute('action') || window.location.pathname;
						var formData = new FormData(form);
						var params = new URLSearchParams(formData).toString();
						var fullUrl = action + (params ? '?' + params : '');
						var targetProxy = toProxyUrl(fullUrl);
						if (targetProxy) {
							e.preventDefault();
							e.stopPropagation();
							window.location.href = targetProxy;
						}
					}, true);

					document.addEventListener('htmx:configRequest', function(evt) {
						var targetProxy = toProxyUrl(evt.detail.path);
						if (targetProxy) {
							evt.detail.path = targetProxy;
						}
					});

					var origPushState = history.pushState;
					var origReplaceState = history.replaceState;
					history.pushState = function(state, title, url) {
						if (url) {
							var targetProxy = toProxyUrl(url);
							if (targetProxy) {
								window.location.href = targetProxy;
								return;
							}
						}
						return origPushState.apply(this, arguments);
					};
					history.replaceState = function(state, title, url) {
						if (url) {
							var targetProxy = toProxyUrl(url);
							if (targetProxy) {
								window.location.href = targetProxy;
								return;
							}
						}
						return origReplaceState.apply(this, arguments);
					};
				})();
			</script>
		`, themeCSS)

		if idx := strings.Index(htmlStr, "<head>"); idx != -1 {
			htmlStr = htmlStr[:idx+6] + injectedCode + htmlStr[idx+6:]
		} else {
			htmlStr = injectedCode + htmlStr
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(htmlStr))
		return nil
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
	return nil
}
