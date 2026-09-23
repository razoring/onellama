package ui

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/format"
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

	// Serve locally installed models that lack an ollama.com library page.
	if strings.HasPrefix(path, "local/") {
		return s.localModelPage(w, r, path, theme)
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

/* Pricing card + blockquotes */
.pricing-block { border-color:#333 !important; }
blockquote { color:#a3a3a3 !important; border-left-color:#333 !important; }

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
.border-neutral-100, .border-neutral-200, .border-gray-100, .border-gray-200, .border-gray-300 { border-color:#333 !important; }
				.divide-gray-100 > :not([hidden]) ~ :not([hidden]), .divide-gray-200 > :not([hidden]) ~ :not([hidden]), .divide-gray-300 > :not([hidden]) ~ :not([hidden]) { border-color:#333 !important; }

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
				body.ol-model header nav > a { display: block !important; }
				body.ol-model header nav .flex-grow { display: none !important; }
				footer { display: none !important; }
				header nav > a[href="/"] img, header a[href="/"] img { width:56px !important; height:56px !important; }
				body { margin-top: 0 !important; padding-top: 0 !important; }
				.ol-installed-btn .ol-trash-icon { display: none !important; }
				.ol-installed-btn .ol-check-icon { display: block !important; }
				.ol-installed-btn:hover .ol-trash-icon { display: block !important; }
				.ol-installed-btn:hover .ol-check-icon { display: none !important; }
			</style>
%[1]s
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

					function applyModelHeader() {
						var p = ((new URLSearchParams(window.location.search)).get('path') || '').replace(/^\/+/, '');
						var isSearch = p === '' || p === 'search';
						document.body.classList.toggle('ol-model', !isSearch);
						var brand = document.querySelector('header nav > a[href="/"]');
						if (brand) {
							var brandImg = brand.querySelector('img');
							if (brandImg && brandImg.getAttribute('src') !== '%[2]s') {
								brandImg.setAttribute('src', '%[2]s');
							}
							brand.addEventListener('click', function(e) {
								e.preventDefault();
								e.stopPropagation();
								window.location.href = window.location.origin + '/api/v1/models/webview?path=search&theme=' + encodeURIComponent(getCurrentTheme());
							});
						}

						var copyright = document.getElementById('ol-copyright');
						if (!copyright) {
							copyright = document.createElement('div');
							copyright.id = 'ol-copyright';
							var copySource = '';
							var copyEls = document.querySelectorAll('footer div');
							for (var i = 0; i < copyEls.length; i++) {
								if (copyEls[i].children.length === 0 && copyEls[i].textContent.indexOf('©') !== -1) {
									copySource = copyEls[i].textContent.trim();
									break;
								}
							}
							copyright.textContent = copySource ? copySource : '© 2026 Ollama';
							copyright.style.position = 'fixed';
							copyright.style.right = '16px';
							copyright.style.bottom = '8px';
							copyright.style.fontSize = '13px';
							copyright.style.zIndex = '9999';
							copyright.style.pointerEvents = 'none';
							copyright.style.color = getCurrentTheme() === 'dark' ? '#a3a3a3' : '#8f8f8f';
							document.body.appendChild(copyright);
						}
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
						// onellama: in "Installed" mode, the search bar filters local models instead of searching the catalog.
						if (installed.enabled() && e.isTrusted && (typeof form.querySelector === 'function') && form.querySelector('input[name="q"]')) {
							e.preventDefault();
							e.stopPropagation();
							return;
						}
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
						// onellama: while the sort is set to "Installed", keep the page in the local installed view (no catalog navigation).
						if (installed.enabled() && evt.detail.path && evt.detail.path.indexOf('/search') === 0) {
							evt.preventDefault();
							return;
						}
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

					// ---------- onellama: "Installed" sort option (client-side catalog view) ----------
					var installed = {
						term: '',
						slugs: [],
						rendered: [],

						sortSelects: function() {
							return Array.prototype.slice.call(document.querySelectorAll('#desktop-sort-select, #mobile-sort-select'));
						},

						enabled: function() {
							var sels = this.sortSelects();
							for (var i = 0; i < sels.length; i++) if (sels[i].value === 'installed') return true;
							return false;
						},

						ensureOptions: function() {
							var sels = this.sortSelects();
							for (var i = 0; i < sels.length; i++) {
								if (sels[i].querySelector('option[value="installed"]')) continue;
								var opt = document.createElement('option');
								opt.value = 'installed';
								opt.textContent = 'Installed';
								sels[i].appendChild(opt);
							}
						},

						loadInstalled: function() {
							var self = this;
							return fetch(window.location.origin + '/api/tags', { headers: { 'Accept': 'application/json' } })
								.then(function(r) { if (!r.ok) throw new Error('http ' + r.status); return r.json(); })
								.then(function(data) {
									var models = (data && data.models) || [];
									var map = {};
									self.names = {};
									for (var i = 0; i < models.length; i++) {
										var mName = models[i].name || models[i].model || '';
										var b = self.normalize(mName);
										if (b) {
											map[b] = true;
											if (!self.names[b]) self.names[b] = mName;
										}
									}
									self.slugs = Object.keys(map);
									return self.slugs;
								})
								.catch(function() { self.slugs = []; self.names = {}; return []; });
						},

						liWrap: function(node) {
							var li = document.createElement('li');
							li.className = 'flex items-baseline border-b border-neutral-200 py-6';
							li.setAttribute('data-ol-installed', '1');
							li.appendChild(node);
							return li;
						},

						fallbackLi: function(base) {
							var li = document.createElement('li');
							li.className = 'flex items-baseline border-b border-neutral-200 py-6';
							li.setAttribute('data-ol-installed', '1');
							var a = document.createElement('a');
							a.href = '/local/library/' + encodeURIComponent(base);
							a.className = 'group w-full';
							var div = document.createElement('div');
							div.className = 'flex flex-col w-full';
							var h = document.createElement('h2');
							h.className = 'truncate text-xl font-medium underline-offset-2 group-hover:underline md:text-2xl';
							h.textContent = this.names[base] || base;
							var p = document.createElement('p');
							p.className = 'text-sm text-neutral-500';
							p.textContent = 'Available Locally. Not available in the ollama library.';
							div.appendChild(h);
							div.appendChild(p);
							a.appendChild(div);
							li.appendChild(a);
							return li;
						},

						cards: function(root) {
							return Array.prototype.slice.call((root || document).querySelectorAll('#searchresults a[href*="/library/"], a[href*="/library/"]'));
						},

						renderedCards: function() {
							return Array.prototype.slice.call(document.querySelectorAll('#searchresults [data-ol-installed]'));
						},

						normalize: function(name) {
							if (!name) return '';
							var s = String(name).trim().toLowerCase();
							var i = s.lastIndexOf('/');
							if (i !== -1) s = s.slice(i + 1);
							i = s.indexOf(':');
							if (i !== -1) s = s.slice(0, i);
							return s;
						},

						matches: function(slug) {
							if (!slug || !this.slugs.length) return false;
							var s = String(slug).trim().toLowerCase();
							var i = s.lastIndexOf('/');
							var sBase = i !== -1 ? s.slice(i + 1) : s;
							for (var j = 0; j < this.slugs.length; j++) {
								var b = this.slugs[j];
								if (b === s || b === sBase) return true;
								if (b.indexOf(sBase) === 0 && b.charAt(sBase.length) === '-') return true;
							}
							return false;
						},

						searchValue: function() {
							var el = document.querySelector('#form-input') || document.querySelector('#navbar-input');
							return el ? (el.value || '') : '';
						},

						fetchCards: function() {
							var self = this;
							var slugs = [];
							for (var i = 0; i < this.slugs.length; i++) if (slugs.indexOf(this.slugs[i]) === -1) slugs.push(this.slugs[i]);
							if (!slugs.length) return Promise.resolve([]);
							var theme = getCurrentTheme();
							return Promise.all(slugs.map(function(s) {
								var u = window.location.origin + '/api/v1/models/webview?path=' + encodeURIComponent('search?q=' + encodeURIComponent(s)) + '&theme=' + encodeURIComponent(theme);
								return fetch(u).then(function(r) { if (!r.ok) throw 0; return r.text(); }).catch(function() { return ''; });
							})).then(function(htmls) {
								var seen = {};
								var found = {};
								var out = [];
								for (var h = 0; h < htmls.length; h++) {
									if (!htmls[h]) continue;
									var dom = new DOMParser().parseFromString(htmls[h], 'text/html');
									var lis = self.cards(dom);
									for (var i = 0; i < lis.length; i++) {
										var href = lis[i].getAttribute('href') || '';
										var m = href.match(/\/library\/([^?#]+)/);
										if (!m || !self.matches(m[1])) continue;
										var b2 = self.normalize(m[1]);
										if (b2) found[b2] = true;
										if (seen[href]) continue;
										seen[href] = true;
										out.push(self.liWrap(lis[i].cloneNode(true)));
									}
								}
								for (var k = 0; k < slugs.length; k++) {
									if (!found[slugs[k]]) out.push(self.fallbackLi(slugs[k]));
								}
								return out;
							});
						},

						showHint: function(msg) {
							this.removeHint();
							var hint = document.createElement('div');
							hint.id = 'ol-installed-hint';
							hint.textContent = msg;
							hint.style.cssText = 'text-align:center;color:#6b7280;padding:48px 16px;font-size:14px;';
							var results = document.getElementById('searchresults');
							if (results) results.appendChild(hint);
						},

						removeHint: function() {
							var h = document.getElementById('ol-installed-hint');
							if (h) h.parentNode.removeChild(h);
						},

						clearRendered: function() {
							var list = this.renderedCards();
							for (var i = 0; i < list.length; i++) list[i].parentNode.removeChild(list[i]);
						},

						render: function() {
							if (!this.enabled()) return;
							this.clearRendered();
							this.removeHint();
							var results = document.getElementById('searchresults');
							if (!results) return;
							this.removeInstalledUl();
							var ul = document.createElement('ul');
							ul.id = 'ol-installed-list';
							ul.className = 'grid grid-cols-1';
							var term = this.term.toLowerCase().trim();
							var visible = 0;
							for (var i = 0; i < this.rendered.length; i++) {
								var node = this.rendered[i];
								var show = true;
								if (term) show = (node.textContent || '').toLowerCase().indexOf(term) !== -1;
								node.style.display = show ? '' : 'none';
								ul.appendChild(node);
								if (show) visible++;
							}
							results.appendChild(ul);
							if (visible === 0) {
								this.showHint(this.rendered.length === 0 ? 'No installed models.' : 'No installed models match "' + this.term.trim() + '".');
							}
						},

						removeInstalledUl: function() {
							var ul = document.getElementById('ol-installed-list');
							if (ul && ul.parentNode) ul.parentNode.removeChild(ul);
						},

						enable: function() {
							var self = this;
							var ul = document.querySelector('#searchresults ul');
							if (ul) { this.origList = ul; ul.style.display = 'none'; }
							this.clearRendered();
							this.removeHint();
							this.term = this.searchValue();
							var ing = document.createElement('div');
							ing.id = 'ol-installed-hint';
							ing.textContent = 'Loading installed models…';
							ing.style.cssText = 'text-align:center;color:#6b7280;padding:48px 16px;font-size:14px;';
							var results = document.getElementById('searchresults');
							if (results) results.appendChild(ing);

							this.loadInstalled().then(function() {
								return self.fetchCards();
							}).then(function(cards) {
								if (!self.enabled()) return;
								self.rendered = cards;
								if (cards.length) self.render(); else self.showHint('No models are installed.');
							}).catch(function() {
								if (self.enabled()) self.showHint('Failed to load installed models.');
							});
						},

						disable: function() {
							this.term = '';
							this.rendered = [];
							this.clearRendered();
							this.removeInstalledUl();
							this.removeHint();
							if (this.origList) { this.origList.style.display = ''; this.origList = null; }
						}
					};

					document.addEventListener('change', function(e) {
						var t = e.target;
						if (!t || (t.id !== 'desktop-sort-select' && t.id !== 'mobile-sort-select')) return;
						if (t.value === 'installed') installed.enable();
						else installed.disable();
					}, true);

					document.addEventListener('input', function(e) {
						var t = e.target;
						if (!t || (t.id !== 'navbar-input' && t.id !== 'form-input')) return;
						if (installed.enabled()) { installed.term = t.value || ''; installed.render(); }
					}, true);

					document.addEventListener('keydown', function(e) {
						if (!installed.enabled()) return;
						var t = e.target;
						if (t && (t.id === 'navbar-input' || t.id === 'form-input') && (e.key === 'Enter' || e.keyCode === 13)) {
							e.preventDefault();
							e.stopPropagation();
						}
					}, true);

					document.addEventListener('htmx:afterSwap', function() { installed.disable(); installed.ensureOptions(); applyModelHeader(); }, true);
					document.addEventListener('DOMContentLoaded', function() { installed.ensureOptions(); applyModelHeader(); });

					installed.loadInstalled();

					// ---------- onellama: Download column injector for model tables ----------
					var CURRENT_OS = "%[3]s";
					var dlColumn = {
						installedMap: {},
						pullingMap: {},
						deletingMap: {},

						loadInstalled: function() {
							var self = this;
							return fetch(window.location.origin + '/api/tags', { headers: { 'Accept': 'application/json' } })
								.then(function(r) { if (!r.ok) throw new Error('http ' + r.status); return r.json(); })
								.then(function(data) {
									var models = (data && data.models) || [];
									self.installedMap = {};
									for (var i = 0; i < models.length; i++) {
										var full = models[i].name || models[i].model || '';
										var norm = self.normalizeTag(full);
										if (norm) self.installedMap[norm] = true;
									}
									try {
										for (var k = 0; k < localStorage.length; k++) {
											var key = localStorage.key(k);
											if (!key) continue;
											if (key.indexOf('onellama_pulling_') === 0) {
												var pTag = key.slice(17);
												if (self.isInstalled(pTag)) {
													localStorage.removeItem(key);
													delete self.pullingMap[pTag];
												}
											} else if (key.indexOf('onellama_deleting_') === 0) {
												var dTag = key.slice(18);
												if (!self.isInstalled(dTag)) {
													localStorage.removeItem(key);
													delete self.deletingMap[dTag];
												}
											}
										}
									} catch (e) {}
									return self.installedMap;
								})
								.catch(function() { self.installedMap = {}; return {}; });
						},

						normalizeTag: function(tag) {
							if (!tag) return '';
							var s = String(tag).trim().toLowerCase();
							s = s.replace(/^ollama\s+(run|pull)\s+/i, '').trim();
							if (s.indexOf('/library/') === 0) s = s.slice(9);
							if (s.indexOf('/') === 0) s = s.slice(1);
							var slashIdx = s.lastIndexOf('/');
							if (slashIdx !== -1 && (s.indexOf('localhost') !== -1 || s.indexOf('ollama.com') !== -1)) {
								s = s.slice(slashIdx + 1);
							}
							return s;
						},

						isInstalled: function(tag) {
							var norm = this.normalizeTag(tag);
							if (!norm) return false;
							if (this.installedMap[norm]) return true;
							if (norm.indexOf(':') === -1) {
								if (this.installedMap[norm + ':latest']) return true;
							} else if (norm.indexOf(':latest') !== -1) {
								var base = norm.slice(0, norm.indexOf(':latest'));
								if (this.installedMap[base]) return true;
							}
							return false;
						},

						isPullingTag: function(tag) {
							var norm = this.normalizeTag(tag);
							if (!tag) return false;
							if (this.pullingMap[tag] || (norm && this.pullingMap[norm])) return true;
							try {
								return localStorage.getItem('onellama_pulling_' + tag) === '1' || (norm && localStorage.getItem('onellama_pulling_' + norm) === '1');
							} catch (e) { return false; }
						},

						isDeletingTag: function(tag) {
							var norm = this.normalizeTag(tag);
							if (!tag) return false;
							if (this.deletingMap[tag] || (norm && this.deletingMap[norm])) return true;
							try {
								return localStorage.getItem('onellama_deleting_' + tag) === '1' || (norm && localStorage.getItem('onellama_deleting_' + norm) === '1');
							} catch (e) { return false; }
						},

						isMLX: function(tag) {
							if (!tag) return false;
							return String(tag).toLowerCase().indexOf('mlx') !== -1;
						},

						isCloud: function(tag, rowElem) {
							if (!tag) return false;
							var s = String(tag).toLowerCase();
							if (s.endsWith(':cloud') || s.indexOf('cloud') !== -1) return true;
							if (rowElem) {
								var txt = (rowElem.textContent || '').toLowerCase();
								if (txt.indexOf('cloud') !== -1) {
									var chips = rowElem.querySelectorAll('.chip, span');
									for (var i = 0; i < chips.length; i++) {
										if (chips[i].textContent.trim().toLowerCase() === 'cloud') return true;
									}
								}
							}
							return false;
						},

						isDownloadable: function(tag, rowElem, isLocalUnlisted) {
							if (!tag) return false;
							if (this.isMLX(tag) && (CURRENT_OS === 'windows' || CURRENT_OS === 'linux')) return false;
							if (this.isCloud(tag, rowElem)) return false;
							if (isLocalUnlisted) return false;
							return true;
						},

						pullModel: function(tag) {
							var self = this;
							var norm = self.normalizeTag(tag);
							self.pullingMap[tag] = true;
							if (norm) self.pullingMap[norm] = true;
							try {
								localStorage.setItem('onellama_pulling_' + tag, '1');
								if (norm) localStorage.setItem('onellama_pulling_' + norm, '1');
							} catch (e) {}
							self.updateUI();
							
							window.parent.postMessage({ type: 'pullModel', tag: tag }, '*');
						},

						deleteModel: function(tag) {
							var self = this;
							var norm = self.normalizeTag(tag);
							self.deletingMap[tag] = true;
							if (norm) self.deletingMap[norm] = true;
							try {
								localStorage.setItem('onellama_deleting_' + tag, '1');
								if (norm) localStorage.setItem('onellama_deleting_' + norm, '1');
							} catch (e) {}
							self.updateUI();
							fetch(window.location.origin + '/api/delete', {
								method: 'DELETE',
								headers: { 'Content-Type': 'application/json' },
								body: JSON.stringify({ name: tag })
							})
							.then(function(r) { if (!r.ok) throw new Error('delete failed'); return r.text(); })
							.then(function() {
								delete self.deletingMap[tag];
								if (norm) delete self.deletingMap[norm];
								try {
									localStorage.removeItem('onellama_deleting_' + tag);
									if (norm) localStorage.removeItem('onellama_deleting_' + norm);
								} catch (e) {}
								return self.loadInstalled();
							})
							.then(function() { self.updateUI(); })
							.catch(function() {
								delete self.deletingMap[tag];
								if (norm) delete self.deletingMap[norm];
								try {
									localStorage.removeItem('onellama_deleting_' + tag);
									if (norm) localStorage.removeItem('onellama_deleting_' + norm);
								} catch (e) {}
								self.updateUI();
							});
						},

						whiteDownloadSVG: '<svg class="h-4 w-4 text-white hover:opacity-80 transition-opacity" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5M12 3v13.5m0 0l-4.5-4.5m4.5 4.5l4.5-4.5" /></svg>',
						graySpinnerSVG: '<svg class="animate-spin h-4 w-4 text-neutral-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>',
						grayCheckSVG: '<svg class="h-4 w-4 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" /></svg>',
						whiteTrashSVG: '<svg class="h-4 w-4 text-white hover:text-red-400 transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" /></svg>',

						getBaseModel: function() {
							var pathParam = new URLSearchParams(window.location.search).get('path') || window.location.pathname;
							pathParam = pathParam.replace(/^\/+/, '').replace(/^library\//, '');
							var parts = pathParam.split('/');
							if (parts[0] && parts[0] !== 'search' && parts[0] !== 'tags') {
								return parts[0].split('?')[0];
							}
							return '';
						},

						extractTag: function(row, baseModel) {
							var tag = '';

							// 1. Check inputs
							var inputs = row.querySelectorAll('input');
							for (var i = 0; i < inputs.length; i++) {
								var val = (inputs[i].value || '').trim();
								val = val.replace(/^ollama\s+(run|pull)\s+/i, '').trim();
								if (val) {
									tag = val;
									break;
								}
							}

							// 2. Check links
							if (!tag) {
								var links = row.querySelectorAll('a[href*="/library/"]');
								for (var l = 0; l < links.length; l++) {
									var href = links[l].getAttribute('href') || '';
									var m = href.match(/\/library\/([^?#]+)/);
									if (m && m[1] && m[1] !== 'tags' && m[1].indexOf('search') === -1) {
										tag = m[1];
										break;
									}
								}
							}

							// 3. Check first cell / link / text if tag is still empty
							if (!tag) {
								var firstCol = row.querySelector('.col-span-6, .col-span-5, a, p, span');
								if (firstCol) {
									var txt = (firstCol.textContent || '').trim().split(/\s+/)[0];
									if (txt && txt.length < 50 && txt.indexOf('Available') === -1 && txt.indexOf('View') === -1) {
										tag = txt;
									}
								}
							}

							if (!tag) return '';
							if (tag.indexOf(' ') !== -1) return '';
							tag = tag.replace(/^[:\/]+/, '');

							if (baseModel && tag.indexOf(':') === -1 && tag.indexOf('/') === -1) {
								tag = baseModel + ':' + tag;
							}

							return tag;
						},

						updateUI: function() {
							var self = this;
							var baseModel = self.getBaseModel();

							// Find top-level header rows
							var allHeaders = document.querySelectorAll('.divide-y > .grid, .table-head, [class*="grid-cols-"]');
							var headerRows = [];
							for (var i = 0; i < allHeaders.length; i++) {
								var el = allHeaders[i];
								var txt = el.textContent || '';
								if (txt.indexOf('Input') === -1 && txt.indexOf('Context') === -1 && txt.indexOf('Size') === -1 && txt.indexOf('Quantization') === -1) continue;

								var isNested = false;
								for (var j = 0; j < headerRows.length; j++) {
									if (headerRows[j].contains(el)) { isNested = true; break; }
								}
								if (!isNested) {
									headerRows.push(el);
								}
							}

							for (var h = 0; h < headerRows.length; h++) {
								var header = headerRows[h];

								// 1. Header column
								if (!header.querySelector('.ol-dl-header')) {
									var p = document.createElement('p');
									p.className = 'col-span-1 hidden md:block ol-dl-header font-medium text-xs text-neutral-900 dark:text-neutral-100 text-center';
									p.textContent = 'Download';

									var children = header.children;
									var inputElem = null;
									for (var c = 0; c < children.length; c++) {
										var cText = children[c].textContent.trim();
										if (cText === 'Input' || cText === 'Quantization' || cText === 'Context' || cText === 'Size / Usage') {
											inputElem = children[c];
										}
									}
									if (inputElem) {
										inputElem.parentNode.insertBefore(p, inputElem.nextSibling);
									} else {
										header.appendChild(p);
									}

									for (var c = 0; c < children.length; c++) {
										if (children[c].classList.contains('col-span-6')) {
											children[c].classList.remove('col-span-6');
											children[c].classList.add('col-span-5');
										}
									}
								}

								// 2. Table container rows
								var tableContainer = header.closest('.divide-y') || header.parentNode;
								if (!tableContainer) continue;

								var rowElems = tableContainer.children;
								for (var r = 0; r < rowElems.length; r++) {
									var row = rowElems[r];
									if (row === header || row.classList.contains('ol-dl-header')) continue;

									var tag = self.extractTag(row, baseModel);
									if (!tag) continue;

									var isLocalUnlisted = (row.textContent || '').indexOf('Available Locally. Not available in the ollama library.') !== -1;
									var gridRow = row.querySelector('.grid') || (row.classList.contains('grid') ? row : null);
									if (!gridRow) {
										gridRow = row.querySelector('.hidden.md\\:flex, .row-desktop, [class*="grid-cols-"]') || row;
									}

									var nameSpan = gridRow.querySelector('.col-span-6');
									if (nameSpan) {
										nameSpan.classList.remove('col-span-6');
										nameSpan.classList.add('col-span-5');
									}

									// Deduplicate cells in the row
									var allDlCells = row.querySelectorAll('.ol-dl-cell');
									var dlCell = allDlCells[0] || null;
									for (var k = 1; k < allDlCells.length; k++) {
										allDlCells[k].parentNode.removeChild(allDlCells[k]);
									}

									if (!dlCell) {
										dlCell = document.createElement('div');
										dlCell.className = 'col-span-1 text-neutral-500 text-[13px] ol-dl-cell flex items-center justify-center text-center';
										gridRow.appendChild(dlCell);
									} else if (dlCell.parentNode !== gridRow) {
										gridRow.appendChild(dlCell);
									}

									var isPulling = self.isPullingTag(tag);
									var isDeleting = self.isDeletingTag(tag);
									var installed = self.isInstalled(tag);
									var downloadable = self.isDownloadable(tag, row, isLocalUnlisted);

									var targetState = (isPulling || isDeleting) ? 'spinner' : (installed ? 'installed' : (!downloadable ? 'dash' : 'download'));

									if (dlCell.getAttribute('data-ol-state') === targetState && dlCell.getAttribute('data-ol-tag') === tag) {
										continue;
									}

									dlCell.setAttribute('data-ol-state', targetState);
									dlCell.setAttribute('data-ol-tag', tag);
									dlCell.innerHTML = '';

									if (targetState === 'spinner') {
										dlCell.innerHTML = self.graySpinnerSVG;
									} else if (targetState === 'installed') {
										var btn = document.createElement('button');
										btn.type = 'button';
										btn.className = 'ol-installed-btn';
										btn.style.cssText = 'background: transparent !important; background-color: transparent !important; border: none !important; outline: none !important; box-shadow: none !important; padding: 4px; cursor: pointer; display: flex; align-items: center; justify-content: center;';
										btn.title = 'Delete model (' + tag + ')';
										btn.innerHTML = '<span class="ol-check-icon">' + self.grayCheckSVG + '</span><span class="ol-trash-icon">' + self.whiteTrashSVG + '</span>';

										(function(mTag) {
											btn.addEventListener('click', function(e) {
												e.preventDefault();
												e.stopPropagation();
												self.deleteModel(mTag);
											});
										})(tag);
										dlCell.appendChild(btn);
									} else if (targetState === 'dash') {
										var dash = document.createElement('span');
										dash.className = 'text-neutral-500 font-medium px-2';
										dash.textContent = '-';
										dlCell.appendChild(dash);
									} else {
										var btn = document.createElement('button');
										btn.type = 'button';
										btn.style.cssText = 'background: transparent !important; background-color: transparent !important; border: none !important; outline: none !important; box-shadow: none !important; padding: 4px; cursor: pointer; display: flex; align-items: center; justify-content: center;';
										btn.title = 'Download model (' + tag + ')';
										btn.innerHTML = self.whiteDownloadSVG;
										(function(mTag) {
											btn.addEventListener('click', function(e) {
												e.preventDefault();
												e.stopPropagation();
												self.pullModel(mTag);
											});
										})(tag);
										dlCell.appendChild(btn);
									}
								}
							}
						},

						init: function() {
							var self = this;
							window.addEventListener('message', function(e) {
								if (e.data && e.data.type === 'pullComplete') {
									var tag = e.data.tag;
									var norm = self.normalizeTag(tag);
									delete self.pullingMap[tag];
									if (norm) delete self.pullingMap[norm];
									self.loadInstalled().then(function() { self.updateUI(); });
								}
							});
							self.loadInstalled().then(function() { self.updateUI(); });
							document.addEventListener('htmx:afterSwap', function() {
								self.loadInstalled().then(function() { self.updateUI(); });
							});
							document.addEventListener('DOMContentLoaded', function() {
								self.loadInstalled().then(function() { self.updateUI(); });
							});
							setInterval(function() { self.updateUI(); }, 1000);
						}
					};

					dlColumn.init();
				})();
			</script>
		`, themeCSS, ollamaLogoDataURI, runtime.GOOS)

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

// localModelPage renders a generic page for a locally installed model that has no ollama.com library page.
func (s *Server) localModelPage(w http.ResponseWriter, r *http.Request, path, theme string) error {
	name := normalizeLocalModelName(strings.TrimPrefix(strings.TrimPrefix(path, "local/"), "library/"))
	if name == "" {
		http.Error(w, "model not found", http.StatusNotFound)
		return nil
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, scheme+"://"+r.Host+"/api/tags", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, fmt.Sprintf("upstream returned %d", resp.StatusCode), resp.StatusCode)
		return nil
	}

	var data struct {
		Models []api.ListModelResponse `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		http.Error(w, "failed to decode tags", http.StatusInternalServerError)
		return nil
	}

	var found *api.ListModelResponse
	for i := range data.Models {
		m := &data.Models[i]
		if normalizeLocalModelName(m.Name) == name || normalizeLocalModelName(m.Model) == name {
			found = m
			break
		}
	}
	if found == nil {
		http.Error(w, "model not found", http.StatusNotFound)
		return nil
	}

	// Remove the DENY header set by the ui.go handler wrapper so this can be iframed.
	w.Header().Del("X-Frame-Options")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, localModelPageHTML(found, theme))
	return nil
}

// normalizeLocalModelName maps a full model tag to the catalog-style slug used for local/library URLs.
func normalizeLocalModelName(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	if i := strings.LastIndex(n, "/"); i != -1 {
		n = n[i+1:]
	}
	if i := strings.Index(n, ":"); i != -1 {
		n = n[:i]
	}
	return n
}

// localModelPageHTML renders a generic page for a locally installed model, styled to match the
// ollama.com library pages in both dark and light themes.
func localModelPageHTML(m *api.ListModelResponse, theme string) string {
	dark := theme == "" || theme == "dark"

	bg, fg, muted, accent, border, cardBg, headBg, codeFg, proseFg := "#ffffff", "#171717", "#8f8f8f", "#3f3f3f", "#e5e5e5", "#ffffff", "#f9fafb", "#3f3f3f", "#4b5563"
	chipIndBg, chipIndFg, chipCynBg, chipCynFg := "#eef2ff", "#4f46e5", "#ecfeff", "#0891b2"
	themeParam := "light"
	if dark {
		bg, fg, muted, accent, border, cardBg, headBg, codeFg, proseFg = "#171717", "#f5f5f5", "#a3a3a3", "#f5f5f5", "#333333", "#1a1a1a", "#1f1f1f", "#d4d4d4", "#d4d4d4"
		chipIndBg, chipIndFg, chipCynBg, chipCynFg = "rgba(99,102,241,0.15)", "#c7d2fe", "rgba(34,211,238,0.15)", "#67e8f9"
		themeParam = "dark"
	}

	name := m.Name
	if name == "" {
		name = m.Model
	}
	display := html.EscapeString(name)

	family := m.Details.Family
	if family == "" && len(m.Details.Families) > 0 {
		family = strings.Join(m.Details.Families, ", ")
	}

	size := format.HumanBytes(m.Size)
	params := html.EscapeString(m.Details.ParameterSize)
	quant := html.EscapeString(m.Details.QuantizationLevel)

	rows := ""
	for _, r := range []struct{ k, v string }{
		{"Family", family},
		{"Parameters", m.Details.ParameterSize},
		{"Quantization", m.Details.QuantizationLevel},
		{"Size", size},
		{"Context", fmt.Sprintf("%d tokens", m.Details.ContextLength)},
		{"Modified", m.ModifiedAt.Format("Jan 2, 2006")},
	} {
		if r.v == "" || r.v == "0 tokens" {
			continue
		}
		rows += `<div class="row"><div class="row-left"><span class="lbl">` + html.EscapeString(r.k) + `</span><code class="inl">` + html.EscapeString(r.v) + `</code></div><button class="copy" type="button" data-copy="` + html.EscapeString(r.v) + `" title="Copy">` + copyIcon + `</button></div>`
	}
	if rows == "" {
		rows = `<div class="row"><div class="row-left"><span class="lbl">No details available</span></div></div>`
	}

	chips := ""
	for i := range m.Capabilities {
		cap := string(m.Capabilities[i])
		kind := "c-indigo"
		switch cap {
		case "vision", "audio", "image", "text", "multimodal", "cloud":
			kind = "c-cyan"
		}
		chips += `<span class="chip ` + kind + `">` + html.EscapeString(cap) + `</span>`
	}

	tableBody := `<div class="row-desktop"><span class="name-cell">` + display + `</span><p>` + size + `</p><p>` + params + `</p><p>` + quant + `</p></div>`
	tableBody += `<div class="row-mobile"><span class="n">` + display + `</span><span class="m">` + size + ` · ` + params + ` · ` + quant + `</span></div>`

	runCode := html.EscapeString("ollama run " + name)
	curlCode := html.EscapeString(`curl http://localhost:11434/api/generate -d '{"model":"` + name + `","prompt":"Why is the sky blue?"}'`)

	searchURL := "/api/v1/models/webview?path=search&theme=" + themeParam

	return fmt.Sprintf(localPageTemplate,
		display,           // 1  title
		size,              // 2  meta size
		muted,             // 3  muted text
		bg,                // 4  page background
		themeParam,        // 5  color-scheme
		fg,                // 6  foreground
		border,            // 7  card/row borders
		headBg,            // 8  table header background
		accent,            // 9  hover/copied accent
		codeFg,            // 10 code text
		proseFg,           // 11 prose text
		chipIndBg,         // 12 indigo chip background
		chipIndFg,         // 13 indigo chip text
		chipCynBg,         // 14 cyan chip background
		chipCynFg,         // 15 cyan chip text
		cardBg,            // 16 card background
		searchURL,         // 17 back link target
		copyIcon,          // 18 copy button glyph
		chips,             // 19 capability chips
		rows,              // 20 details rows
		tableBody,         // 21 installed-version table body
		runCode,           // 22 run snippet
		curlCode,          // 23 api snippet
		ollamaLogoDataURI) // 24 brand logo (data URI)
}

// ollamaLogoDataURI is the high-DPI Ollama circle logo (ollama.com /public/icon-64x64.png:
// white filled circle with black outline at 64x64), embedded so it renders crisp at 56px on
// the local model page and is swapped onto the cloud pages' header brand image.
const ollamaLogoDataURI = "data:image/png;base64,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"

// copyIcon is the inline copy glyph used on the card/row buttons (matches the ollama.com pages).
const copyIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`

const localPageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%[1]s · Ollama</title>
<style>
:root{color-scheme:%[5]s}
*{box-sizing:border-box}
html{height:100vh}
body{margin:0;background:%[4]s;color:%[6]s;font-family:ui-sans-serif,system-ui,-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;line-height:1.5;color-scheme:%[5]s;-webkit-font-smoothing:antialiased}
.wrap{max-width:832px;margin:0 auto;padding:40px 24px 96px;width:100%%}
@media(min-width:640px){.wrap{padding-top:80px}}
@media(min-width:1024px){.wrap{padding-left:32px;padding-right:32px}}
.brand-nav{margin:0 0 28px;display:flex;justify-content:center}
.brand-nav a{display:inline-flex;line-height:0}
.brand-nav img{width:56px;height:auto;display:block}
h1{margin:0 0 8px;color:%[6]s;font-size:20px;font-weight:500;letter-spacing:-.7px;line-height:1.5;overflow-wrap:break-word}
@media(min-width:640px){h1{font-size:28px}}
.meta{margin:0;color:%[3]s;font-size:13px;font-weight:500}
.chips{display:flex;flex-wrap:wrap;gap:8px;margin:28px 0 0}
.chip{display:inline-flex;align-items:center;border-radius:6px;padding:2px 8px;font-size:12px;font-weight:500;line-height:20px}
@media(min-width:640px){.chip{font-size:13px;padding:3px 10px}}
.c-indigo{background:%[12]s;color:%[13]s}
.c-cyan{background:%[14]s;color:%[15]s}
.spacer{height:32px}
.block{margin:0 0 32px}
.h2{margin:0 0 16px;color:%[6]s;font-size:16px;font-weight:600;line-height:24px}
.card{border:1px solid %[7]s;border-radius:8px;overflow:hidden;background:%[16]s}
.card-bar{display:flex;align-items:center;justify-content:space-between;padding:4px 12px 0 7px}
.tabs{display:flex}
.tab{border:0;background:none;cursor:pointer;padding:8px 12px;font-size:12px;font-weight:500;color:%[3]s;font-family:inherit;line-height:1}
.tab.on{color:%[6]s;text-decoration:underline;text-decoration-thickness:1px;text-underline-offset:7px}
.tab:hover{color:%[9]s}
.copy{border:0;background:none;cursor:pointer;padding:6px;border-radius:6px;color:%[3]s;display:inline-flex;align-items:center;justify-content:center}
.copy:hover{color:%[9]s}
.copy.copied{color:%[9]s !important;font-size:12px;font-family:inherit}
.panel{padding:16px;overflow-x:auto}
.panel.hidden{display:none}
.pre{margin:0;color:%[10]s;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px;white-space:pre-wrap;overflow-wrap:anywhere}
.row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 16px;border-top:1px solid %[7]s}
.row:first-child{border-top:0}
.row-left{display:flex;align-items:center;gap:12px;min-width:0}
.lbl{color:%[3]s;font-size:14px;white-space:nowrap}
code.inl{color:%[6]s;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px;overflow-wrap:anywhere}
.table-head{display:grid;grid-template-columns:repeat(12,1fr);background:%[8]s;padding:12px 16px;font-size:12px;color:%[6]s}
.col-6{grid-column:span 6 / span 6}
.col-2{grid-column:span 2 / span 2}
.row-desktop{display:none;grid-template-columns:repeat(12,1fr);padding:12px 16px;font-size:13px;border-top:1px solid %[7]s}
@media(min-width:640px){.row-desktop{display:grid}}
.name-cell{grid-column:span 6 / span 6;color:%[6]s;overflow-wrap:anywhere;padding-right:12px}
.row-desktop p{margin:0;color:%[3]s;overflow-wrap:anywhere}
.row-mobile{display:flex;flex-direction:column;gap:6px;padding:12px 16px;font-size:13px;border-top:1px solid %[7]s}
@media(min-width:640px){.row-mobile{display:none}}
.row-mobile .n{color:%[6]s;overflow-wrap:anywhere}
.row-mobile .m{color:%[3]s;overflow-wrap:anywhere}
.prose p{margin:0;color:%[11]s;font-size:16px}
</style>
</head>
<body>
<div class="wrap">
<nav class="brand-nav"><a href="%[17]s" title="Back to search" aria-label="Back to search"><img src="%[24]s" alt="Ollama"></a></nav>
<header>
<h1>%[1]s</h1>
<p class="meta">Installed locally · %[2]s</p>
<div class="chips">%[19]s</div>
</header>
<div class="spacer"></div>
<section class="block">
<div class="card">
<div class="card-bar">
<div class="tabs">
<button class="tab on" type="button">Run</button>
<button class="tab" type="button">API</button>
</div>
<button class="copy" type="button" data-copy="%[22]s" title="Copy">%[18]s</button>
</div>
<div class="panel" data-panel><pre class="pre">%[22]s</pre></div>
<div class="panel hidden" data-panel><pre class="pre">%[23]s</pre></div>
</div>
</section>
<section class="block">
<h2 class="h2">Details</h2>
<div class="card">%[20]s</div>
</section>
<section class="block">
<h2 class="h2">Installed version</h2>
<div class="card">
<div class="table-head"><span class="col-6">Name</span><span class="col-2">Size</span><span class="col-2">Parameters</span><span class="col-2">Quantization</span></div>
%[21]s
</div>
</section>
<section class="block prose">
<p>This model is installed locally and has no Ollama library page, so the details above reflect this installation. Chat with it from the top of the app, or run it from your terminal: <code class="inl">ollama run %[1]s</code></p>
</section>
</div>
<script>
(function () {
  var ICON = '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>';
  var tabs = document.querySelectorAll('.tab');
  var panels = document.querySelectorAll('[data-panel]');
  for (var i = 0; i < tabs.length; i++) (function (btn, pi) {
    btn.addEventListener('click', function () {
      for (var j = 0; j < tabs.length; j++) tabs[j].classList.remove('on');
      for (var j = 0; j < panels.length; j++) panels[j].classList.add('hidden');
      btn.classList.add('on');
      panels[pi].classList.remove('hidden');
    });
  })(tabs[i], i);
  var copies = document.querySelectorAll('.copy');
  for (var i = 0; i < copies.length; i++) (function (btn) {
    btn.addEventListener('click', function () {
      var txt = btn.getAttribute('data-copy') || '';
      function feedback() {
        btn.classList.add('copied');
        btn.textContent = 'Copied';
        setTimeout(function () { btn.classList.remove('copied'); btn.innerHTML = ICON; }, 1200);
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(txt);
        feedback();
      } else {
        var ta = document.createElement('textarea'); ta.value = txt; document.body.appendChild(ta); ta.select();
        try { document.execCommand('copy'); } catch (e) {}
        document.body.removeChild(ta);
        feedback();
      }
    });
  })(copies[i]);
})();
</script>
</body>
</html>`
