//go:build windows || darwin

package tools

import (
	"strings"
	"testing"
)

func TestRegistrySystemPrompt(t *testing.T) {
	r := NewRegistry()
	sched := NewSchedulerTool(nil)
	r.Register(sched)

	prompt := r.SystemPrompt()
	if !strings.Contains(prompt, "Current Local Time:") {
		t.Errorf("expected SystemPrompt to contain Current Local Time, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Action-Oriented & Decisive") {
		t.Errorf("expected SystemPrompt to contain Action-Oriented guidelines, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Handling Scheduled & Recurring Requests") {
		t.Errorf("expected SystemPrompt to contain scheduling instructions, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Scheduler") {
		t.Errorf("expected SystemPrompt to contain Scheduler tool name, got: %s", prompt)
	}
}
