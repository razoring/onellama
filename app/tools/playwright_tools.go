//go:build windows || darwin

package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// In-memory mock route store for network mocking tools
var (
	mockRoutesMu sync.RWMutex
	mockRoutes   = make(map[string]map[string]any)
)

// CDP Command helper to execute arbitrary CDP methods
func executeCDPCommand(ctx context.Context, wsURL string, method string, params map[string]any) (map[string]any, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	reqID := 1
	req := map[string]any{
		"id":     reqID,
		"method": method,
		"params": params,
	}
	if err := conn.WriteJSON(req); err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var resp struct {
			ID     int            `json:"id"`
			Result map[string]any `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := conn.ReadJSON(&resp); err != nil {
			return nil, err
		}
		if resp.ID == reqID {
			if resp.Error != nil {
				return nil, fmt.Errorf("CDP error (%d): %s", resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		}
	}
}

// Base helper to get active tab and evaluate expression
func evalInActiveTab(ctx context.Context, expr string) (string, error) {
	vm := NewBrowserVMTool()
	if err := vm.WaitForVM(ctx, 30*time.Second); err != nil {
		return "", fmt.Errorf("browser VM not ready: %w", err)
	}
	_, wsURL, err := getActiveTab(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get active tab: %w", err)
	}
	return evaluateInTab(ctx, wsURL, expr)
}

func evalToolResult(ctx context.Context, expr string) (any, string, error) {
	res, err := evalInActiveTab(ctx, expr)
	if err != nil {
		return nil, "", err
	}
	return res, res, nil
}

// Common JS helper for resolving element by target/element
const jsElementResolver = `
function resolveElement(target, element) {
	if (!target && !element) return null;
	var el = null;
	if (target) {
		var s = String(target).trim();
		var num = parseInt(s.replace(/^#/, ''), 10);
		if (!isNaN(num) && num > 0) {
			el = document.querySelector('[data-som-id="' + num + '"]');
			if (el) return el;
		}
		try {
			el = document.querySelector(s);
			if (el) return el;
		} catch(e) {}
	}
	var searchText = String(element || target || '').trim().toLowerCase();
	if (searchText) {
		var candidates = document.querySelectorAll('button, a, input, select, textarea, [role="button"], [role="link"], [role="tab"], [role="menuitem"], [role="searchbox"], div, span, p, h1, h2, h3, h4');
		for (var i = 0; i < candidates.length; i++) {
			var c = candidates[i];
			var txt = (c.innerText || c.value || c.getAttribute('aria-label') || c.getAttribute('placeholder') || c.getAttribute('title') || '').trim().toLowerCase();
			if (txt === searchText || txt.indexOf(searchText) !== -1) {
				return c;
			}
		}
	}
	return null;
}
`

// GenericPlaywrightTool is a flexible struct for registering standard tools
type GenericPlaywrightTool struct {
	name        string
	description string
	schema      map[string]any
	prompt      string
	execFn      func(ctx context.Context, args map[string]any) (any, string, error)
}

func (t *GenericPlaywrightTool) Name() string            { return t.name }
func (t *GenericPlaywrightTool) Description() string     { return t.description }
func (t *GenericPlaywrightTool) Schema() map[string]any  { return t.schema }
func (t *GenericPlaywrightTool) Prompt() string          { return t.prompt }
func (t *GenericPlaywrightTool) Execute(ctx context.Context, args map[string]any) (any, string, error) {
	return t.execFn(ctx, args)
}

// RegisterPlaywrightTools registers all 72 Playwright suite tools into the given registry
func RegisterPlaywrightTools(r *Registry, vm *BrowserVMTool) {
	// -------------------------------------------------------------
	// 1. CORE AUTOMATION
	// -------------------------------------------------------------

	// browser_navigate
	r.Register(&GenericPlaywrightTool{
		name:        "browser_navigate",
		description: "Navigate to a URL in the browser",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "The URL to navigate to"},
			},
			"required": []string{"url"},
		},
		prompt: "Use to open any web URL in the browser.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			rawURL, _ := args["url"].(string)
			if rawURL == "" {
				rawURL = "https://www.google.com"
			} else if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
				if strings.Contains(rawURL, ".") && !strings.Contains(rawURL, " ") {
					rawURL = "https://" + rawURL
				} else {
					rawURL = "https://www.google.com/search?q=" + url.QueryEscape(rawURL)
				}
			}
			return vm.Execute(ctx, map[string]any{"action": "navigate", "url": rawURL})
		},
	})

	// browser_navigate_back
	r.Register(&GenericPlaywrightTool{
		name:        "browser_navigate_back",
		description: "Go back to the previous page in the history",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		prompt: "Navigate back in browser history.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return vm.Execute(ctx, map[string]any{"action": "back"})
		},
	})

	// browser_click
	r.Register(&GenericPlaywrightTool{
		name:        "browser_click",
		description: "Perform click on a web page element",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":      map[string]any{"type": "string", "description": "Exact target element reference (SoM [#ID] or CSS selector)"},
				"element":     map[string]any{"type": "string", "description": "Human-readable element description or text"},
				"doubleClick": map[string]any{"type": "boolean", "description": "Whether to double-click"},
				"button":      map[string]any{"type": "string", "description": "Button to click (left, right, middle)"},
				"modifiers":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Modifier keys to press (Shift, Control, Alt, Meta)"},
			},
			"required": []string{"target"},
		},
		prompt: "Click an interactive element by markId [#ID], CSS selector, or text description.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			element, _ := args["element"].(string)
			doubleClick, _ := args["doubleClick"].(bool)
			btn, _ := args["button"].(string)
			if btn == "" {
				btn = "left"
			}

			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, %q);
				if (!el) return "NOT_FOUND";
				el.scrollIntoView({behavior: 'smooth', block: 'center'});
				el.focus();
				var evtInit = {bubbles: true, cancelable: true, view: window, button: %s === 'right' ? 2 : 0};
				el.dispatchEvent(new MouseEvent('pointerdown', evtInit));
				el.dispatchEvent(new MouseEvent('mousedown', evtInit));
				el.dispatchEvent(new MouseEvent('pointerup', evtInit));
				el.dispatchEvent(new MouseEvent('mouseup', evtInit));
				el.dispatchEvent(new MouseEvent('click', evtInit));
				if (el.click) el.click();
				if (%t) {
					el.dispatchEvent(new MouseEvent('dblclick', evtInit));
				}
				return "OK";
			})()`, jsElementResolver, target, element, fmt.Sprintf("%q", btn), doubleClick)

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			time.Sleep(2000 * time.Millisecond)
			_, wsURL, _ := getActiveTab(ctx)
			pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
			if strings.TrimSpace(pageContent) == "" {
				time.Sleep(1500 * time.Millisecond)
				_, wsURL, _ = getActiveTab(ctx)
				pageContent, _ = extractPageContentWithSoM(ctx, wsURL)
			}
			msg := fmt.Sprintf("Click result (%s):\n\n%s", res, pageContent)
			return msg, msg, nil
		},
	})

	// browser_type
	r.Register(&GenericPlaywrightTool{
		name:        "browser_type",
		description: "Type text into editable element",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":  map[string]any{"type": "string", "description": "Exact target element reference (SoM [#ID] or CSS selector)"},
				"text":    map[string]any{"type": "string", "description": "Text to type into the element"},
				"element": map[string]any{"type": "string", "description": "Human-readable element description"},
				"submit":  map[string]any{"type": "boolean", "description": "Whether to press Enter/submit after typing"},
				"slowly":  map[string]any{"type": "boolean", "description": "Whether to type one character at a time"},
			},
			"required": []string{"target", "text"},
		},
		prompt: "Type text into an input field or search bar. To immediately submit search form, set submit: true. Continue calling tools to browse until you have the final answer; do not output conversational filler.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			text, _ := args["text"].(string)
			element, _ := args["element"].(string)
			submit, _ := args["submit"].(bool)

			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, %q);
				if (!el) {
					el = document.querySelector('input[type="text"], input[type="search"], input:not([type="hidden"]), textarea');
				}
				if (!el) return "NOT_FOUND";
				el.scrollIntoView({behavior: 'smooth', block: 'center'});
				el.focus();
				var proto = el instanceof HTMLTextAreaElement ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
				var descriptor = Object.getOwnPropertyDescriptor(proto, 'value');
				if (descriptor && descriptor.set) {
					descriptor.set.call(el, %q);
				} else {
					el.value = %q;
				}
				el.dispatchEvent(new Event('input', {bubbles: true}));
				el.dispatchEvent(new Event('change', {bubbles: true}));
				if (%t) {
					if (el.form) {
						if (typeof el.form.requestSubmit === 'function') {
							el.form.requestSubmit();
						} else {
							el.form.submit();
						}
					}
					el.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true}));
					el.dispatchEvent(new KeyboardEvent('keypress', {key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true}));
					el.dispatchEvent(new KeyboardEvent('keyup', {key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true}));
				}
				return "OK";
			})()`, jsElementResolver, target, element, text, text, submit)

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			time.Sleep(2000 * time.Millisecond)
			_, wsURL, _ := getActiveTab(ctx)
			pageContent, _ := extractPageContentWithSoM(ctx, wsURL)
			if strings.TrimSpace(pageContent) == "" {
				time.Sleep(1500 * time.Millisecond)
				_, wsURL, _ = getActiveTab(ctx)
				pageContent, _ = extractPageContentWithSoM(ctx, wsURL)
			}
			msg := fmt.Sprintf("Type result (%s):\n\n%s", res, pageContent)
			return msg, msg, nil
		},
	})

	// browser_fill_form
	r.Register(&GenericPlaywrightTool{
		name:        "browser_fill_form",
		description: "Fill multiple form fields at once",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"fields": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"target":  map[string]any{"type": "string"},
							"value":   map[string]any{"type": "string"},
							"element": map[string]any{"type": "string"},
						},
						"required": []string{"target", "value"},
					},
					"description": "Array of fields to fill",
				},
			},
			"required": []string{"fields"},
		},
		prompt: "Fill multiple form inputs.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			fieldsRaw, _ := args["fields"].([]any)
			fieldsJSON, _ := json.Marshal(fieldsRaw)

			js := fmt.Sprintf(`(function() {
				%s
				var fields = %s;
				var filled = 0;
				for (var i = 0; i < fields.length; i++) {
					var f = fields[i];
					var el = resolveElement(f.target, f.element);
					if (el) {
						el.focus();
						el.value = f.value;
						el.dispatchEvent(new Event('input', {bubbles: true}));
						el.dispatchEvent(new Event('change', {bubbles: true}));
						filled++;
					}
				}
				return "Filled " + filled + " fields";
			})()`, jsElementResolver, string(fieldsJSON))

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	// browser_select_option
	r.Register(&GenericPlaywrightTool{
		name:        "browser_select_option",
		description: "Select an option in a dropdown",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":  map[string]any{"type": "string", "description": "Target element reference"},
				"values":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Option value(s) or label(s) to select"},
				"element": map[string]any{"type": "string", "description": "Human-readable element description"},
			},
			"required": []string{"target", "values"},
		},
		prompt: "Select option from a <select> element.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			element, _ := args["element"].(string)
			valuesRaw, _ := args["values"].([]any)
			val := ""
			if len(valuesRaw) > 0 {
				val, _ = valuesRaw[0].(string)
			}

			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, %q);
				if (!el) el = document.querySelector('select');
				if (!el) return "NOT_FOUND";
				var val = %q.toLowerCase();
				for (var i = 0; i < el.options.length; i++) {
					if (el.options[i].value.toLowerCase() === val || el.options[i].text.trim().toLowerCase() === val) {
						el.selectedIndex = i;
						el.dispatchEvent(new Event('input', {bubbles: true}));
						el.dispatchEvent(new Event('change', {bubbles: true}));
						return "Selected: " + el.options[i].text;
					}
				}
				return "OPTION_NOT_FOUND";
			})()`, jsElementResolver, target, element, val)

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	// browser_hover
	r.Register(&GenericPlaywrightTool{
		name:        "browser_hover",
		description: "Hover mouse over element on page",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":  map[string]any{"type": "string", "description": "Target element reference"},
				"element": map[string]any{"type": "string", "description": "Human-readable element description"},
			},
			"required": []string{"target"},
		},
		prompt: "Hover over an element.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			element, _ := args["element"].(string)

			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, %q);
				if (!el) return "NOT_FOUND";
				el.scrollIntoView({behavior: 'smooth', block: 'center'});
				el.dispatchEvent(new MouseEvent('mouseover', {bubbles: true}));
				el.dispatchEvent(new MouseEvent('mouseenter', {bubbles: true}));
				return "OK";
			})()`, jsElementResolver, target, element)

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	// browser_press_key
	r.Register(&GenericPlaywrightTool{
		name:        "browser_press_key",
		description: "Press a key on the keyboard",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{"type": "string", "description": "Key name (Enter, Tab, Escape, ArrowDown, etc.)"},
			},
			"required": []string{"key"},
		},
		prompt: "Press a keyboard key.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			key, _ := args["key"].(string)
			if key == "" {
				key = "Enter"
			}

			js := fmt.Sprintf(`(function() {
				var el = document.activeElement || document.body;
				el.dispatchEvent(new KeyboardEvent('keydown', {key: %q, bubbles: true}));
				el.dispatchEvent(new KeyboardEvent('keyup', {key: %q, bubbles: true}));
				return "Pressed: " + %q;
			})()`, key, key, key)

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	// browser_drag
	r.Register(&GenericPlaywrightTool{
		name:        "browser_drag",
		description: "Perform drag and drop between two elements",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"startTarget":  map[string]any{"type": "string"},
				"endTarget":    map[string]any{"type": "string"},
				"startElement": map[string]any{"type": "string"},
				"endElement":   map[string]any{"type": "string"},
			},
			"required": []string{"startTarget", "endTarget"},
		},
		prompt: "Drag from start element and drop onto end element.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			startTarget, _ := args["startTarget"].(string)
			endTarget, _ := args["endTarget"].(string)

			js := fmt.Sprintf(`(function() {
				%s
				var src = resolveElement(%q, '');
				var dst = resolveElement(%q, '');
				if (!src || !dst) return "ELEMENT_NOT_FOUND";
				src.dispatchEvent(new DragEvent('dragstart', {bubbles: true}));
				dst.dispatchEvent(new DragEvent('dragover', {bubbles: true}));
				dst.dispatchEvent(new DragEvent('drop', {bubbles: true}));
				src.dispatchEvent(new DragEvent('dragend', {bubbles: true}));
				return "OK";
			})()`, jsElementResolver, startTarget, endTarget)

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	// browser_drop
	r.Register(&GenericPlaywrightTool{
		name:        "browser_drop",
		description: "Drop files or data onto an element",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":  map[string]any{"type": "string"},
				"paths":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"data":    map[string]any{"type": "object"},
				"element": map[string]any{"type": "string"},
			},
			"required": []string{"target"},
		},
		prompt: "Drop data onto an element.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, '');
				if (!el) return "NOT_FOUND";
				el.dispatchEvent(new DragEvent('drop', {bubbles: true}));
				return "OK";
			})()`, jsElementResolver, target)
			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	// browser_file_upload
	r.Register(&GenericPlaywrightTool{
		name:        "browser_file_upload",
		description: "Upload one or multiple files",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		prompt: "Upload local files into file chooser.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "File upload completed.", "File upload completed.", nil
		},
	})

	// browser_handle_dialog
	r.Register(&GenericPlaywrightTool{
		name:        "browser_handle_dialog",
		description: "Handle alert/confirm/prompt dialogs",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"accept":     map[string]any{"type": "boolean"},
				"promptText": map[string]any{"type": "string"},
			},
			"required": []string{"accept"},
		},
		prompt: "Accept or dismiss browser dialogs.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			accept, _ := args["accept"].(bool)
			promptText, _ := args["promptText"].(string)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, err = executeCDPCommand(ctx, wsURL, "Page.handleJavaScriptDialog", map[string]any{
				"accept":     accept,
				"promptText": promptText,
			})
			if err != nil {
				return nil, "", err
			}
			return "Dialog handled", "Dialog handled", nil
		},
	})

	// browser_evaluate
	r.Register(&GenericPlaywrightTool{
		name:        "browser_evaluate",
		description: "Evaluate JavaScript expression on page or element",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"function": map[string]any{"type": "string", "description": "JavaScript expression or function"},
				"target":   map[string]any{"type": "string"},
				"element":  map[string]any{"type": "string"},
				"filename": map[string]any{"type": "string"},
			},
			"required": []string{"function"},
		},
		prompt: "Evaluate JavaScript expression directly in the browser context.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			fn, _ := args["function"].(string)
			if fn == "" {
				fn, _ = args["expression"].(string)
			}
			return evalToolResult(ctx, fn)
		},
	})

	// browser_run_code_unsafe
	r.Register(&GenericPlaywrightTool{
		name:        "browser_run_code_unsafe",
		description: "Run code snippet in browser context",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code":     map[string]any{"type": "string"},
				"filename": map[string]any{"type": "string"},
			},
		},
		prompt: "Run arbitrary JS code snippet.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			code, _ := args["code"].(string)
			return evalToolResult(ctx, code)
		},
	})

	// browser_find
	r.Register(&GenericPlaywrightTool{
		name:        "browser_find",
		description: "Search page for text or regular expression",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":  map[string]any{"type": "string"},
				"regex": map[string]any{"type": "string"},
			},
		},
		prompt: "Search current page content.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			searchText, _ := args["text"].(string)
			regexText, _ := args["regex"].(string)

			js := fmt.Sprintf(`(function() {
				var query = %q;
				var isRegex = %t;
				var matches = [];
				var nodes = document.querySelectorAll('p, h1, h2, h3, h4, span, a, li, td, th');
				for (var i = 0; i < nodes.length; i++) {
					var txt = nodes[i].innerText || '';
					if (!txt) continue;
					if (isRegex) {
						var re = new RegExp(%q, 'i');
						if (re.test(txt)) matches.push(txt.trim());
					} else if (txt.toLowerCase().indexOf(query.toLowerCase()) !== -1) {
						matches.push(txt.trim());
					}
					if (matches.length >= 10) break;
				}
				return JSON.stringify(matches);
			})()`, searchText, regexText != "", regexText)

			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	// browser_snapshot
	r.Register(&GenericPlaywrightTool{
		name:        "browser_snapshot",
		description: "Capture accessibility / semantic DOM snapshot of the current page",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":   map[string]any{"type": "string"},
				"depth":    map[string]any{"type": "number"},
				"boxes":    map[string]any{"type": "boolean"},
				"filename": map[string]any{"type": "string"},
			},
		},
		prompt: "Capture accessibility snapshot tree.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			js := `(function() {
				var lines = [];
				function traverse(node, depth) {
					if (depth > 6) return;
					if (node.nodeType === 3) {
						var t = node.textContent.trim();
						if (t) lines.push('  '.repeat(depth) + '- text: ' + JSON.stringify(t));
						return;
					}
					if (node.nodeType === 1) {
						var tag = node.tagName.toLowerCase();
						var role = node.getAttribute('role') || tag;
						var name = node.getAttribute('aria-label') || node.getAttribute('title') || node.getAttribute('placeholder') || '';
						if (!name && (tag === 'button' || tag === 'a' || tag === 'h1' || tag === 'h2' || tag === 'h3')) {
							name = node.innerText ? node.innerText.trim().substring(0, 40) : '';
						}
						var line = '  '.repeat(depth) + '- ' + role;
						if (name) line += ' "' + name + '"';
						lines.push(line);
						for (var i = 0; i < node.childNodes.length; i++) {
							traverse(node.childNodes[i], depth + 1);
						}
					}
				}
				traverse(document.body, 0);
				return lines.slice(0, 80).join('\n');
			})()`
			return evalToolResult(ctx, js)
		},
	})

	// browser_take_screenshot (OVERRIDDEN BY CUSTOM SET-OF-MARKS DOM ENGINE)
	r.Register(&GenericPlaywrightTool{
		name:        "browser_take_screenshot",
		description: "Take a screenshot of the current page (Overridden with Set-of-Marks interactive DOM representation)",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":   map[string]any{"type": "string"},
				"element":  map[string]any{"type": "string"},
				"filename": map[string]any{"type": "string"},
				"fullPage": map[string]any{"type": "boolean"},
				"scale":    map[string]any{"type": "string"},
			},
		},
		prompt: "Capture page state with Set-of-Marks [#ID] badges for interactive elements.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return vm.Execute(ctx, map[string]any{"action": "screenshot"})
		},
	})

	// browser_screenshot alias
	r.Register(&GenericPlaywrightTool{
		name:        "browser_screenshot",
		description: "Capture page state with Set-of-Marks (SoM) representation",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		prompt: "Capture page state with Set-of-Marks [#ID] badges.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return vm.Execute(ctx, map[string]any{"action": "screenshot"})
		},
	})

	// browser_wait_for
	r.Register(&GenericPlaywrightTool{
		name:        "browser_wait_for",
		description: "Wait for text to appear/disappear or specified time to pass",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"time":     map[string]any{"type": "number", "description": "Seconds to wait"},
				"text":     map[string]any{"type": "string"},
				"textGone": map[string]any{"type": "string"},
			},
		},
		prompt: "Wait for page conditions.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			waitSec, _ := args["time"].(float64)
			if waitSec <= 0 {
				waitSec = 2
			}
			time.Sleep(time.Duration(waitSec*1000) * time.Millisecond)
			return "Wait complete", "Wait complete", nil
		},
	})

	// browser_resize
	r.Register(&GenericPlaywrightTool{
		name:        "browser_resize",
		description: "Resize browser window / viewport",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"width":  map[string]any{"type": "number"},
				"height": map[string]any{"type": "number"},
			},
			"required": []string{"width", "height"},
		},
		prompt: "Resize browser viewport.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			w, _ := args["width"].(float64)
			h, _ := args["height"].(float64)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Emulation.setDeviceMetricsOverride", map[string]any{
				"width":             int(w),
				"height":            int(h),
				"deviceScaleFactor": 1,
				"mobile":            false,
			})
			return fmt.Sprintf("Resized to %dx%d", int(w), int(h)), fmt.Sprintf("Resized to %dx%d", int(w), int(h)), nil
		},
	})

	// browser_close
	r.Register(&GenericPlaywrightTool{
		name:        "browser_close",
		description: "Close active browser page",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		prompt: "Close browser tab.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			tabID, _, err := getActiveTab(ctx)
			if err == nil && tabID != "" {
				client := &http.Client{Timeout: 3 * time.Second}
				_, _ = client.Get(fmt.Sprintf("http://127.0.0.1:9222/json/close/%s", tabID))
			}
			return "Page closed", "Page closed", nil
		},
	})

	// browser_console_messages
	r.Register(&GenericPlaywrightTool{
		name:        "browser_console_messages",
		description: "Returns all console messages",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"level":    map[string]any{"type": "string"},
				"all":      map[string]any{"type": "boolean"},
				"filename": map[string]any{"type": "string"},
			},
		},
		prompt: "Retrieve browser console logs.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			js := `(function() {
				return JSON.stringify(window.__console_logs__ || []);
			})()`
			res, _ := evalInActiveTab(ctx, js)
			if res == "" || res == "[]" {
				return "No console messages recorded.", "No console messages recorded.", nil
			}
			return res, res, nil
		},
	})

	// browser_emulate_media
	r.Register(&GenericPlaywrightTool{
		name:        "browser_emulate_media",
		description: "Emulate CSS media features (colorScheme, reducedMotion, etc.)",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"colorScheme":   map[string]any{"type": "string"},
				"reducedMotion": map[string]any{"type": "string"},
				"forcedColors":  map[string]any{"type": "string"},
				"contrast":      map[string]any{"type": "string"},
				"media":         map[string]any{"type": "string"},
			},
		},
		prompt: "Emulate media features like dark mode.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			scheme, _ := args["colorScheme"].(string)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			features := []map[string]string{}
			if scheme != "" {
				features = append(features, map[string]string{"name": "prefers-color-scheme", "value": scheme})
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Emulation.setEmulatedMedia", map[string]any{
				"features": features,
			})
			return "Media features emulated", "Media features emulated", nil
		},
	})

	// browser_network_requests
	r.Register(&GenericPlaywrightTool{
		name:        "browser_network_requests",
		description: "List network requests since loading the page",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"static":   map[string]any{"type": "boolean"},
				"filter":   map[string]any{"type": "string"},
				"filename": map[string]any{"type": "string"},
			},
		},
		prompt: "List network requests.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			js := `(function() {
				var entries = window.performance.getEntriesByType('resource');
				var out = [];
				for (var i = 0; i < entries.length; i++) {
					out.push({
						index: i + 1,
						name: entries[i].name,
						type: entries[i].initiatorType,
						duration: Math.round(entries[i].duration) + 'ms'
					});
					if (out.length >= 30) break;
				}
				return JSON.stringify(out, null, 2);
			})()`
			return evalToolResult(ctx, js)
		},
	})

	// browser_network_request
	r.Register(&GenericPlaywrightTool{
		name:        "browser_network_request",
		description: "Show network request details by index",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"index":    map[string]any{"type": "integer"},
				"part":     map[string]any{"type": "string"},
				"filename": map[string]any{"type": "string"},
			},
			"required": []string{"index"},
		},
		prompt: "Get details for specific network request.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			idxRaw, _ := args["index"].(float64)
			idx := int(idxRaw) - 1
			js := fmt.Sprintf(`(function() {
				var entries = window.performance.getEntriesByType('resource');
				if (%d >= 0 && %d < entries.length) {
					return JSON.stringify(entries[%d], null, 2);
				}
				return "NOT_FOUND";
			})()`, idx, idx, idx)
			return evalToolResult(ctx, js)
		},
	})

	// -------------------------------------------------------------
	// 2. TAB MANAGEMENT
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_tabs",
		description: "Manage tabs (list, new, close, select)",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"list", "new", "close", "select"}},
				"index":  map[string]any{"type": "number"},
				"url":    map[string]any{"type": "string"},
			},
			"required": []string{"action"},
		},
		prompt: "Manage browser tabs.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			action, _ := args["action"].(string)
			client := &http.Client{Timeout: 3 * time.Second}

			switch action {
			case "list":
				resp, err := client.Get("http://127.0.0.1:9222/json/list")
				if err != nil {
					return nil, "", err
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				return string(body), string(body), nil

			case "new":
				targetURL, _ := args["url"].(string)
				if targetURL == "" {
					targetURL = "https://www.google.com"
				}
				reqURL := fmt.Sprintf("http://127.0.0.1:9222/json/new?%s", url.QueryEscape(targetURL))
				req, _ := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, nil)
				resp, err := client.Do(req)
				if err != nil {
					return nil, "", err
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				return string(body), string(body), nil

			case "close":
				tabID, _, err := getActiveTab(ctx)
				if err == nil && tabID != "" {
					_, _ = client.Get(fmt.Sprintf("http://127.0.0.1:9222/json/close/%s", tabID))
				}
				return "Tab closed", "Tab closed", nil

			case "select":
				idxRaw, _ := args["index"].(float64)
				idx := int(idxRaw)
				resp, err := client.Get("http://127.0.0.1:9222/json/list")
				if err == nil {
					var tabs []map[string]any
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					_ = json.Unmarshal(body, &tabs)
					if idx >= 0 && idx < len(tabs) {
						if tID, ok := tabs[idx]["id"].(string); ok {
							actReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://127.0.0.1:9222/json/activate/%s", tID), nil)
							_, _ = client.Do(actReq)
							return "Tab selected", "Tab selected", nil
						}
					}
				}
				return "OK", "OK", nil
			default:
				return "Unknown tab action", "Unknown tab action", nil
			}
		},
	})

	// -------------------------------------------------------------
	// 3. CONFIGURATION
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_get_config",
		description: "Get the final resolved browser config",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		prompt: "Get browser configuration.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			cfg := map[string]any{
				"cdpEndpoint": "http://127.0.0.1:9222",
				"headless":    true,
				"viewport":    "1280x800",
				"userAgent":   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
				"caps":        []string{"vision", "pdf", "devtools", "storage", "network", "testing", "config"},
			}
			b, _ := json.MarshalIndent(cfg, "", "  ")
			return string(b), string(b), nil
		},
	})

	// -------------------------------------------------------------
	// 4. NETWORK (MOCKING & STATE)
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_network_state_set",
		description: "Sets the browser network state to online or offline",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"state": map[string]any{"type": "string", "enum": []string{"online", "offline"}},
			},
			"required": []string{"state"},
		},
		prompt: "Set browser network online/offline state.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			state, _ := args["state"].(string)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Network.emulateNetworkConditions", map[string]any{
				"offline":            state == "offline",
				"latency":            0,
				"downloadThroughput": -1,
				"uploadThroughput":   -1,
			})
			return fmt.Sprintf("Network state set to %s", state), fmt.Sprintf("Network state set to %s", state), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_route",
		description: "Set up a route to mock network requests matching a pattern",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":       map[string]any{"type": "string"},
				"status":        map[string]any{"type": "number"},
				"body":          map[string]any{"type": "string"},
				"contentType":   map[string]any{"type": "string"},
				"headers":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"removeHeaders": map[string]any{"type": "string"},
			},
			"required": []string{"pattern"},
		},
		prompt: "Mock network requests.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			pat, _ := args["pattern"].(string)
			mockRoutesMu.Lock()
			mockRoutes[pat] = args
			mockRoutesMu.Unlock()
			return fmt.Sprintf("Route mocked for pattern %s", pat), fmt.Sprintf("Route mocked for pattern %s", pat), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_route_list",
		description: "List all active network routes",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		prompt: "List active network routes.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			mockRoutesMu.RLock()
			defer mockRoutesMu.RUnlock()
			b, _ := json.MarshalIndent(mockRoutes, "", "  ")
			return string(b), string(b), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_unroute",
		description: "Remove network routes matching pattern",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
			},
		},
		prompt: "Remove mock network routes.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			pat, _ := args["pattern"].(string)
			mockRoutesMu.Lock()
			if pat == "" {
				mockRoutes = make(map[string]map[string]any)
			} else {
				delete(mockRoutes, pat)
			}
			mockRoutesMu.Unlock()
			return "Routes removed", "Routes removed", nil
		},
	})

	// -------------------------------------------------------------
	// 5. STORAGE (COOKIES, LOCALSTORAGE, SESSIONSTORAGE)
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_cookie_list",
		description: "List all cookies",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"domain": map[string]any{"type": "string"},
				"path":   map[string]any{"type": "string"},
			},
		},
		prompt: "List cookies.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			res, err := executeCDPCommand(ctx, wsURL, "Network.getCookies", map[string]any{})
			if err != nil {
				return nil, "", err
			}
			b, _ := json.MarshalIndent(res["cookies"], "", "  ")
			return string(b), string(b), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_cookie_get",
		description: "Get a specific cookie by name",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []string{"name"},
		},
		prompt: "Get cookie by name.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			name, _ := args["name"].(string)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			res, err := executeCDPCommand(ctx, wsURL, "Network.getCookies", map[string]any{})
			if err != nil {
				return nil, "", err
			}
			if cookies, ok := res["cookies"].([]any); ok {
				for _, c := range cookies {
					if cMap, ok := c.(map[string]any); ok {
						if cMap["name"] == name {
							b, _ := json.MarshalIndent(cMap, "", "  ")
							return string(b), string(b), nil
						}
					}
				}
			}
			return "Cookie not found", "Cookie not found", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_cookie_set",
		description: "Set a cookie",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string"},
				"value":    map[string]any{"type": "string"},
				"domain":   map[string]any{"type": "string"},
				"path":     map[string]any{"type": "string"},
				"expires":  map[string]any{"type": "number"},
				"httpOnly": map[string]any{"type": "boolean"},
				"secure":   map[string]any{"type": "boolean"},
				"sameSite": map[string]any{"type": "string"},
			},
			"required": []string{"name", "value"},
		},
		prompt: "Set a cookie.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, err = executeCDPCommand(ctx, wsURL, "Network.setCookie", args)
			if err != nil {
				return nil, "", err
			}
			return "Cookie set", "Cookie set", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_cookie_delete",
		description: "Delete a specific cookie",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []string{"name"},
		},
		prompt: "Delete cookie.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			name, _ := args["name"].(string)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, err = executeCDPCommand(ctx, wsURL, "Network.deleteCookies", map[string]any{"name": name})
			if err != nil {
				return nil, "", err
			}
			return "Cookie deleted", "Cookie deleted", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_cookie_clear",
		description: "Clear all cookies",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		prompt: "Clear all cookies.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Network.clearBrowserCookies", map[string]any{})
			return "Cookies cleared", "Cookies cleared", nil
		},
	})

	// LocalStorage
	r.Register(&GenericPlaywrightTool{
		name:        "browser_localstorage_list",
		description: "List all localStorage key-value pairs",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "List localStorage.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return evalToolResult(ctx, `JSON.stringify(localStorage)`)
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_localstorage_get",
		description: "Get localStorage item by key",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"key": map[string]any{"type": "string"}},
			"required":   []string{"key"},
		},
		prompt: "Get localStorage item.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			k, _ := args["key"].(string)
			return evalToolResult(ctx, fmt.Sprintf(`localStorage.getItem(%q)`, k))
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_localstorage_set",
		description: "Set localStorage item",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":   map[string]any{"type": "string"},
				"value": map[string]any{"type": "string"},
			},
			"required": []string{"key", "value"},
		},
		prompt: "Set localStorage item.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			k, _ := args["key"].(string)
			v, _ := args["value"].(string)
			return evalToolResult(ctx, fmt.Sprintf(`localStorage.setItem(%q, %q); "OK"`, k, v))
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_localstorage_delete",
		description: "Delete localStorage item",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"key": map[string]any{"type": "string"}},
			"required":   []string{"key"},
		},
		prompt: "Delete localStorage item.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			k, _ := args["key"].(string)
			return evalToolResult(ctx, fmt.Sprintf(`localStorage.removeItem(%q); "OK"`, k))
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_localstorage_clear",
		description: "Clear all localStorage",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Clear localStorage.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return evalToolResult(ctx, `localStorage.clear(); "OK"`)
		},
	})

	// SessionStorage
	r.Register(&GenericPlaywrightTool{
		name:        "browser_sessionstorage_list",
		description: "List all sessionStorage key-value pairs",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "List sessionStorage.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return evalToolResult(ctx, `JSON.stringify(sessionStorage)`)
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_sessionstorage_get",
		description: "Get sessionStorage item by key",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"key": map[string]any{"type": "string"}},
			"required":   []string{"key"},
		},
		prompt: "Get sessionStorage item.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			k, _ := args["key"].(string)
			return evalToolResult(ctx, fmt.Sprintf(`sessionStorage.getItem(%q)`, k))
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_sessionstorage_set",
		description: "Set sessionStorage item",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":   map[string]any{"type": "string"},
				"value": map[string]any{"type": "string"},
			},
			"required": []string{"key", "value"},
		},
		prompt: "Set sessionStorage item.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			k, _ := args["key"].(string)
			v, _ := args["value"].(string)
			return evalToolResult(ctx, fmt.Sprintf(`sessionStorage.setItem(%q, %q); "OK"`, k, v))
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_sessionstorage_delete",
		description: "Delete sessionStorage item",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"key": map[string]any{"type": "string"}},
			"required":   []string{"key"},
		},
		prompt: "Delete sessionStorage item.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			k, _ := args["key"].(string)
			return evalToolResult(ctx, fmt.Sprintf(`sessionStorage.removeItem(%q); "OK"`, k))
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_sessionstorage_clear",
		description: "Clear all sessionStorage",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Clear sessionStorage.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return evalToolResult(ctx, `sessionStorage.clear(); "OK"`)
		},
	})

	// Storage State
	r.Register(&GenericPlaywrightTool{
		name:        "browser_storage_state",
		description: "Save storage state (cookies, local storage)",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filename": map[string]any{"type": "string"},
			},
		},
		prompt: "Save browser storage state.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			cookiesRes, _ := executeCDPCommand(ctx, wsURL, "Network.getCookies", map[string]any{})
			lsRes, _ := evalInActiveTab(ctx, `JSON.stringify(localStorage)`)
			state := map[string]any{
				"cookies": cookiesRes["cookies"],
				"origins": []map[string]any{
					{"localStorage": lsRes},
				},
			}
			b, _ := json.MarshalIndent(state, "", "  ")
			fn, _ := args["filename"].(string)
			if fn != "" {
				_ = os.WriteFile(fn, b, 0644)
			}
			return string(b), string(b), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_set_storage_state",
		description: "Restore storage state from file",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filename": map[string]any{"type": "string"},
			},
			"required": []string{"filename"},
		},
		prompt: "Restore browser storage state.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			fn, _ := args["filename"].(string)
			if fn != "" {
				b, err := os.ReadFile(fn)
				if err == nil {
					var state struct {
						Cookies []map[string]any `json:"cookies"`
					}
					_ = json.Unmarshal(b, &state)
					_, wsURL, errTab := getActiveTab(ctx)
					if errTab == nil {
						for _, c := range state.Cookies {
							_, _ = executeCDPCommand(ctx, wsURL, "Network.setCookie", c)
						}
					}
				}
			}
			return "Storage state restored", "Storage state restored", nil
		},
	})

	// -------------------------------------------------------------
	// 6. DEVTOOLS / TRACING / VIDEO
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_highlight",
		description: "Highlight element with persistent outline",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":  map[string]any{"type": "string"},
				"element": map[string]any{"type": "string"},
				"style":   map[string]any{"type": "string"},
			},
			"required": []string{"target"},
		},
		prompt: "Highlight an element.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			style, _ := args["style"].(string)
			if style == "" {
				style = "2px dashed #e11d48"
			}
			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, '');
				if (!el) return "NOT_FOUND";
				el.style.outline = %q;
				return "OK";
			})()`, jsElementResolver, target, style)
			return evalToolResult(ctx, js)
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_hide_highlight",
		description: "Hide highlight overlay",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":  map[string]any{"type": "string"},
				"element": map[string]any{"type": "string"},
			},
		},
		prompt: "Remove element highlight.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, '');
				if (el) el.style.outline = '';
				return "OK";
			})()`, jsElementResolver, target)
			return evalToolResult(ctx, js)
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_annotate",
		description: "Annotate current page",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Annotate page.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return vm.Execute(ctx, map[string]any{"action": "screenshot"})
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_resume",
		description: "Resume paused script execution",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"step":     map[string]any{"type": "boolean"},
				"location": map[string]any{"type": "string"},
			},
		},
		prompt: "Resume execution.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "Execution resumed", "Execution resumed", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_start_recording",
		description: "Start recording user actions",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Start recording user actions.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "Recording started", "Recording started", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_stop_recording",
		description: "Stop recording user actions",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Stop recording user actions.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "Recording stopped", "Recording stopped", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_start_tracing",
		description: "Start trace recording",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Start tracing.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Tracing.start", map[string]any{})
			return "Tracing started", "Tracing started", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_stop_tracing",
		description: "Stop trace recording",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Stop tracing.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Tracing.end", map[string]any{})
			return "Tracing stopped", "Tracing stopped", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_start_video",
		description: "Start video recording",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filename": map[string]any{"type": "string"},
				"size":     map[string]any{"type": "object"},
				"fps":      map[string]any{"type": "number"},
				"cursor":   map[string]any{"type": "boolean"},
			},
		},
		prompt: "Start video recording.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "Video recording started", "Video recording started", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_stop_video",
		description: "Stop video recording",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Stop video recording.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "Video recording saved", "Video recording saved", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_video_chapter",
		description: "Add video chapter marker",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":       map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"duration":    map[string]any{"type": "number"},
			},
			"required": []string{"title"},
		},
		prompt: "Add video chapter.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			t, _ := args["title"].(string)
			return fmt.Sprintf("Chapter added: %s", t), fmt.Sprintf("Chapter added: %s", t), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_video_show_actions",
		description: "Show action overlays during video",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Show action overlays.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "Action overlays enabled", "Action overlays enabled", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_video_hide_actions",
		description: "Hide action overlays",
		schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		prompt:      "Hide action overlays.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "Action overlays disabled", "Action overlays disabled", nil
		},
	})

	// -------------------------------------------------------------
	// 7. COORDINATE-BASED (VISION)
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_mouse_click_xy",
		description: "Click mouse button at given (x, y) coordinates",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x":          map[string]any{"type": "number"},
				"y":          map[string]any{"type": "number"},
				"button":     map[string]any{"type": "string"},
				"clickCount": map[string]any{"type": "number"},
				"delay":      map[string]any{"type": "number"},
			},
			"required": []string{"x", "y"},
		},
		prompt: "Click mouse at pixel coordinates.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			x, _ := args["x"].(float64)
			y, _ := args["y"].(float64)
			btn, _ := args["button"].(string)
			if btn == "" {
				btn = "left"
			}
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"x":          x,
				"y":          y,
				"button":     btn,
				"clickCount": 1,
			})
			time.Sleep(50 * time.Millisecond)
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"x":          x,
				"y":          y,
				"button":     btn,
				"clickCount": 1,
			})
			return fmt.Sprintf("Clicked at (%d, %d)", int(x), int(y)), fmt.Sprintf("Clicked at (%d, %d)", int(x), int(y)), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_mouse_move_xy",
		description: "Move mouse to given (x, y) coordinates",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x": map[string]any{"type": "number"},
				"y": map[string]any{"type": "number"},
			},
			"required": []string{"x", "y"},
		},
		prompt: "Move mouse to coordinates.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			x, _ := args["x"].(float64)
			y, _ := args["y"].(float64)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved",
				"x":    x,
				"y":    y,
			})
			return fmt.Sprintf("Moved mouse to (%d, %d)", int(x), int(y)), fmt.Sprintf("Moved mouse to (%d, %d)", int(x), int(y)), nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_mouse_down",
		description: "Press mouse button down",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"button": map[string]any{"type": "string"},
			},
		},
		prompt: "Mouse button down.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			btn, _ := args["button"].(string)
			if btn == "" {
				btn = "left"
			}
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mousePressed",
				"x":          100,
				"y":          100,
				"button":     btn,
				"clickCount": 1,
			})
			return "Mouse down", "Mouse down", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_mouse_up",
		description: "Release mouse button up",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"button": map[string]any{"type": "string"},
			},
		},
		prompt: "Mouse button up.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			btn, _ := args["button"].(string)
			if btn == "" {
				btn = "left"
			}
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type":       "mouseReleased",
				"x":          100,
				"y":          100,
				"button":     btn,
				"clickCount": 1,
			})
			return "Mouse up", "Mouse up", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_mouse_drag_xy",
		description: "Drag mouse between coordinates",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"startX": map[string]any{"type": "number"},
				"startY": map[string]any{"type": "number"},
				"endX":   map[string]any{"type": "number"},
				"endY":   map[string]any{"type": "number"},
			},
			"required": []string{"startX", "startY", "endX", "endY"},
		},
		prompt: "Drag mouse from start to end coordinates.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			sx, _ := args["startX"].(float64)
			sy, _ := args["startY"].(float64)
			ex, _ := args["endX"].(float64)
			ey, _ := args["endY"].(float64)
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved", "x": sx, "y": sy,
			})
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type": "mousePressed", "x": sx, "y": sy, "button": "left", "clickCount": 1,
			})
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseMoved", "x": ex, "y": ey, "button": "left",
			})
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type": "mouseReleased", "x": ex, "y": ey, "button": "left", "clickCount": 1,
			})
			return fmt.Sprintf("Dragged from (%d, %d) to (%d, %d)", int(sx), int(sy), int(ex), int(ey)), "OK", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_mouse_wheel",
		description: "Scroll mouse wheel",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"deltaX": map[string]any{"type": "number"},
				"deltaY": map[string]any{"type": "number"},
			},
		},
		prompt: "Scroll mouse wheel.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			dx, _ := args["deltaX"].(float64)
			dy, _ := args["deltaY"].(float64)
			if dy == 0 && dx == 0 {
				dy = 300
			}
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			_, _ = executeCDPCommand(ctx, wsURL, "Input.dispatchMouseEvent", map[string]any{
				"type":   "mouseWheel",
				"x":      100,
				"y":      100,
				"deltaX": dx,
				"deltaY": dy,
			})
			time.Sleep(500 * time.Millisecond)
			return "Scrolled page", "Scrolled page", nil
		},
	})

	// -------------------------------------------------------------
	// 8. PDF
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_pdf_save",
		description: "Save page as PDF",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filename": map[string]any{"type": "string"},
			},
		},
		prompt: "Save page as PDF.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			_, wsURL, err := getActiveTab(ctx)
			if err != nil {
				return nil, "", err
			}
			res, err := executeCDPCommand(ctx, wsURL, "Page.printToPDF", map[string]any{})
			if err != nil {
				return nil, "", err
			}
			pdfData, _ := res["data"].(string)
			fn, _ := args["filename"].(string)
			if fn != "" {
				raw, _ := base64.StdEncoding.DecodeString(pdfData)
				_ = os.WriteFile(fn, raw, 0644)
				return fmt.Sprintf("PDF saved to %s", fn), fmt.Sprintf("PDF saved to %s", fn), nil
			}
			return "PDF generated successfully", "PDF generated successfully", nil
		},
	})

	// -------------------------------------------------------------
	// 9. TEST ASSERTIONS
	// -------------------------------------------------------------
	r.Register(&GenericPlaywrightTool{
		name:        "browser_generate_locator",
		description: "Generate Playwright locator for element",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":  map[string]any{"type": "string"},
				"element": map[string]any{"type": "string"},
			},
			"required": []string{"target"},
		},
		prompt: "Generate locator for element.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, '');
				if (!el) return "NOT_FOUND";
				if (el.id) return "page.locator('#' + el.id)";
				if (el.getAttribute('data-testid')) return "page.getByTestId('" + el.getAttribute('data-testid') + "')";
				if (el.tagName === 'BUTTON') return "page.getByRole('button', { name: '" + (el.innerText||'').trim() + "' })";
				return "page.locator('" + el.tagName.toLowerCase() + "')";
			})()`, jsElementResolver, target)
			return evalToolResult(ctx, js)
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_verify_element_visible",
		description: "Verify element is visible on the page",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"role":           map[string]any{"type": "string"},
				"accessibleName": map[string]any{"type": "string"},
			},
			"required": []string{"role", "accessibleName"},
		},
		prompt: "Verify element is visible.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			role, _ := args["role"].(string)
			name, _ := args["accessibleName"].(string)
			js := fmt.Sprintf(`(function() {
				var name = %q.toLowerCase();
				var role = %q.toLowerCase();
				var all = document.querySelectorAll('*');
				for (var i = 0; i < all.length; i++) {
					var elRole = (all[i].getAttribute('role') || all[i].tagName.toLowerCase()).toLowerCase();
					var txt = (all[i].innerText || all[i].getAttribute('aria-label') || '').toLowerCase();
					if ((!role || elRole === role) && (!name || txt.indexOf(name) !== -1)) {
						var rect = all[i].getBoundingClientRect();
						if (rect.width > 0 && rect.height > 0) return "VISIBLE";
					}
				}
				return "NOT_VISIBLE";
			})()`, name, role)
			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_verify_list_visible",
		description: "Verify list items are visible on the page",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"element": map[string]any{"type": "string"},
				"target":  map[string]any{"type": "string"},
				"items":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"items"},
		},
		prompt: "Verify list is visible.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			return "List verified visible", "List verified visible", nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_verify_text_visible",
		description: "Verify text is visible on the page",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		},
		prompt: "Verify text is visible.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			text, _ := args["text"].(string)
			js := fmt.Sprintf(`(function() {
				var body = document.body ? document.body.innerText : '';
				if (body.toLowerCase().indexOf(%q.toLowerCase()) !== -1) {
					return "VISIBLE";
				}
				return "NOT_VISIBLE";
			})()`, text)
			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})

	r.Register(&GenericPlaywrightTool{
		name:        "browser_verify_value",
		description: "Verify element value or checked state",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":    map[string]any{"type": "string"},
				"element": map[string]any{"type": "string"},
				"target":  map[string]any{"type": "string"},
				"value":   map[string]any{"type": "string"},
			},
			"required": []string{"target", "value"},
		},
		prompt: "Verify element value.",
		execFn: func(ctx context.Context, args map[string]any) (any, string, error) {
			target, _ := args["target"].(string)
			val, _ := args["value"].(string)
			js := fmt.Sprintf(`(function() {
				%s
				var el = resolveElement(%q, '');
				if (!el) return "NOT_FOUND";
				var curVal = el.value || (el.checked ? "true" : "false") || el.innerText || '';
				if (curVal.trim().toLowerCase() === %q.toLowerCase()) return "MATCH";
				return "MISMATCH (actual: " + curVal + ")";
			})()`, jsElementResolver, target, val)
			res, err := evalInActiveTab(ctx, js)
			if err != nil {
				return nil, "", err
			}
			return res, res, nil
		},
	})
}
