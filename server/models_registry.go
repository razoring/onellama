package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// ModelsSearchHandler acts as a proxy/parser for ollama.com/search.
func (s *Server) ModelsSearchHandler(c *gin.Context) {
	q := c.Query("q")
	cap := c.Query("c")
	o := c.Query("o")

	target, err := url.Parse("https://ollama.com/search")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid target url"})
		return
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

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Important to pretend to be a normal browser if they have basic anti-bot,
	// though Ollama API might just allow it.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("upstream returned %d", resp.StatusCode)})
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read upstream body"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"html": string(body),
	})
}

// ModelsWebviewHandler proxies ollama.com pages (search, library detail) and strips X-Frame-Options & navbar.
func (s *Server) ModelsWebviewHandler(c *gin.Context) {
	path := c.Query("path")
	theme := c.Query("theme")

	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "search"
	}

	// Prevent open proxy abuse (only allow paths on ollama.com, forbid scheme/domain injection)
	if strings.Contains(path, "://") || strings.HasPrefix(path, "//") {
		c.Data(http.StatusBadRequest, "text/plain", []byte("invalid path"))
		return
	}

	targetURLStr := fmt.Sprintf("https://ollama.com/%s", path)
	targetURL, err := url.Parse(targetURLStr)
	if err != nil {
		c.Data(http.StatusInternalServerError, "text/plain", []byte(err.Error()))
		return
	}

	targetQuery := targetURL.Query()
	for k, vv := range c.Request.URL.Query() {
		if k == "path" || k == "theme" {
			continue
		}
		for _, v := range vv {
			targetQuery.Add(k, v)
		}
	}
	targetURL.RawQuery = targetQuery.Encode()

	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		c.Data(http.StatusInternalServerError, "text/plain", []byte(err.Error()))
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	
	// Copy HX- headers from the client
	for k, vv := range c.Request.Header {
		if strings.HasPrefix(strings.ToUpper(k), "HX-") {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Data(http.StatusBadGateway, "text/plain", []byte(err.Error()))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Data(http.StatusInternalServerError, "text/plain", []byte("failed to read body"))
		return
	}

	// Strip restrictive headers
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if lk == "x-frame-options" || lk == "content-security-policy" {
			continue
		}
		for _, v := range vv {
			c.Header(k, v)
		}
	}

	htmlStr := string(body)
	contentType := resp.Header.Get("Content-Type")

	// Inject base tag, navbar hiding CSS, theme styles, and link interceptor for HTML responses
	if strings.Contains(contentType, "html") || strings.Contains(htmlStr, "<head>") {
		var themeCSS string
		if theme == "dark" || theme == "" {
			themeCSS = `<style id="onellama-theme">
				:root {
					--bg-primary: #171717 !important;
					--bg-secondary: #171717 !important;
					--bg-tertiary: #262626 !important;
					--text-primary: #f3f4f6 !important;
					--text-secondary: #9ca3af !important;
					--border-color: #262626 !important;
				}

				html, body {
					background-color: #171717 !important;
					color: #f3f4f6 !important;
					color-scheme: dark !important;
				}

				/* Target titles and primary text */
				.text-black, .text-neutral-900, .text-neutral-800, .text-neutral-700,
				h1, h2, h3, h4, h5, h6, strong, b {
					color: #f3f4f6 !important;
				}

				/* Target descriptions and secondary text */
				.text-neutral-600, .text-neutral-500, .text-neutral-400,
				.text-gray-600, .text-gray-500, .text-gray-400,
				.text-slate-600, .text-slate-500, .text-slate-400 {
					color: #9ca3af !important;
				}

				/* Target white/light backgrounds */
				.bg-white, body, main, section {
					background-color: #171717 !important;
				}

				/* Card containers, filters & borders */
				.bg-neutral-50, .bg-neutral-100, .bg-neutral-200,
				.bg-gray-50, .bg-gray-100, .bg-gray-200,
				[class*="border-neutral"], [class*="border-gray"] {
					background-color: #171717 !important;
					border-color: #262626 !important;
				}

				/* Tag pills (tools, thinking, cloud, vision) */
				.bg-neutral-900, .bg-black, [class*="bg-neutral-800"], [class*="bg-neutral-900"] {
					background-color: #1e293b !important;
					color: #60a5fa !important;
					border: 1px solid #334155 !important;
				}

				/* Links */
				a {
					color: #60a5fa !important;
				}
				h1 a, h2 a, h3 a, a.group {
					color: #f3f4f6 !important;
				}

				/* Borders */
				hr, [class*="divide-"] {
					border-color: #262626 !important;
				}

				/* Inputs & Selects */
				input, select, textarea {
					background-color: #171717 !important;
					color: #f3f4f6 !important;
					border-color: #333333 !important;
				}

				/* Headers and Navbar */
				header {
					background-color: #171717 !important;
				}
				header nav .appearance-none {
					background-color: #262626 !important;
					border-color: #333333 !important;
				}
				header nav .appearance-none input {
					background-color: transparent !important;
				}
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
		c.Data(resp.StatusCode, contentType, []byte(htmlStr))
		return
	}

	c.Data(resp.StatusCode, contentType, body)
}

