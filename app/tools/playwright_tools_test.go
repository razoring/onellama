//go:build windows || darwin

package tools

import (
	"testing"
)

func TestPlaywrightToolsRegistration(t *testing.T) {
	r := NewRegistry()
	vm := NewBrowserVMTool()
	r.Register(vm)
	RegisterPlaywrightTools(r, vm)

	expectedTools := []string{
		"browser_navigate",
		"browser_navigate_back",
		"browser_click",
		"browser_type",
		"browser_fill_form",
		"browser_select_option",
		"browser_hover",
		"browser_press_key",
		"browser_drag",
		"browser_drop",
		"browser_file_upload",
		"browser_handle_dialog",
		"browser_evaluate",
		"browser_run_code_unsafe",
		"browser_find",
		"browser_snapshot",
		"browser_take_screenshot",
		"browser_screenshot",
		"browser_wait_for",
		"browser_resize",
		"browser_close",
		"browser_console_messages",
		"browser_emulate_media",
		"browser_network_requests",
		"browser_network_request",
		"browser_tabs",
		"browser_get_config",
		"browser_network_state_set",
		"browser_route",
		"browser_route_list",
		"browser_unroute",
		"browser_cookie_list",
		"browser_cookie_get",
		"browser_cookie_set",
		"browser_cookie_delete",
		"browser_cookie_clear",
		"browser_localstorage_list",
		"browser_localstorage_get",
		"browser_localstorage_set",
		"browser_localstorage_delete",
		"browser_localstorage_clear",
		"browser_sessionstorage_list",
		"browser_sessionstorage_get",
		"browser_sessionstorage_set",
		"browser_sessionstorage_delete",
		"browser_sessionstorage_clear",
		"browser_storage_state",
		"browser_set_storage_state",
		"browser_highlight",
		"browser_hide_highlight",
		"browser_annotate",
		"browser_resume",
		"browser_start_recording",
		"browser_stop_recording",
		"browser_start_tracing",
		"browser_stop_tracing",
		"browser_start_video",
		"browser_stop_video",
		"browser_video_chapter",
		"browser_video_show_actions",
		"browser_video_hide_actions",
		"browser_mouse_click_xy",
		"browser_mouse_move_xy",
		"browser_mouse_down",
		"browser_mouse_up",
		"browser_mouse_drag_xy",
		"browser_mouse_wheel",
		"browser_pdf_save",
		"browser_generate_locator",
		"browser_verify_element_visible",
		"browser_verify_list_visible",
		"browser_verify_text_visible",
		"browser_verify_value",
	}

	for _, name := range expectedTools {
		tool, ok := r.Get(name)
		if !ok {
			t.Errorf("expected tool %s not registered", name)
			continue
		}
		if tool.Description() == "" {
			t.Errorf("tool %s has empty description", name)
		}
		if tool.Schema() == nil {
			t.Errorf("tool %s has nil schema", name)
		}
	}
}
