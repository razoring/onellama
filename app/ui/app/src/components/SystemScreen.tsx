import React, { useEffect, useState } from "react";
import type { Memory } from "@/api";
import {
  getSettings,
  updateSettings,
  fetchMemories,
  updateMemory,
  deleteMemory,
} from "@/api";
import type { Settings } from "@/gotypes";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { CheckIcon } from "@heroicons/react/20/solid";

export default function SystemScreen() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [systemPrompt, setSystemPrompt] = useState("");
  const [memoryEnabled, setMemoryEnabled] = useState(false);
  const [memories, setMemories] = useState<Memory[]>([]);
  const [editingMemory, setEditingMemory] = useState<Memory | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    async function load() {
      try {
        const s = await getSettings();
        setSettings(s.settings);
        setSystemPrompt(s.settings.SystemPrompt || "");
        setMemoryEnabled(s.settings.MemoryEnabled !== false);

        const m = await fetchMemories();
        setMemories(m);
      } catch (err) {
        console.error("Failed to load system settings", err);
      }
    }
    load();
  }, []);

  const handleSaveSettings = async () => {
    if (!settings) return;
    setIsSaving(true);
    setSaveError(null);
    setSaveSuccess(false);
    try {
      await updateSettings({
        ...settings,
        SystemPrompt: systemPrompt,
        MemoryEnabled: memoryEnabled,
      } as any);
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    } catch (err) {
      console.error("Failed to save settings", err);
      setSaveError("Failed to save settings");
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeleteMemory = async (id: string) => {
    try {
      await deleteMemory(id);
      setMemories((prev) => prev.filter((m) => m.id !== id));
    } catch (err) {
      console.error("Failed to delete memory", err);
    }
  };

  const handleUpdateMemory = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingMemory) return;
    try {
      const updated = await updateMemory(
        editingMemory.id,
        editingMemory.content,
        editingMemory.title
      );
      setMemories((prev) =>
        prev.map((m) => (m.id === updated.id ? updated : m))
      );
      setEditingMemory(null);
    } catch (err) {
      console.error("Failed to update memory", err);
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-white text-neutral-900 dark:bg-neutral-900 dark:text-neutral-100 overflow-y-auto p-6 space-y-6">
      {/* Global Settings Block */}
      <div className="flex flex-col gap-5 border-b border-neutral-200 dark:border-neutral-800 pb-6">
        <div className="flex flex-col gap-1.5">
          <label className="text-xs font-semibold text-neutral-700 dark:text-neutral-300">
            Global System Prompt
          </label>
          <textarea
            className="w-full h-28 p-3 rounded-xl border border-neutral-200 bg-neutral-50 dark:border-neutral-800 dark:bg-neutral-950 text-neutral-900 dark:text-neutral-100 text-xs font-sans leading-relaxed shadow-xs focus:outline-2 outline-blue-500 resize-y"
            placeholder="Enter instructions that will apply globally to all agent conversations..."
            value={systemPrompt}
            onChange={(e) => setSystemPrompt(e.target.value)}
          />
        </div>

        <div className="flex items-center justify-between rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4 shadow-xs">
          <div>
            <div className="text-xs font-semibold text-neutral-900 dark:text-neutral-100">
              Persistent Memory
            </div>
            <p className="text-xs text-neutral-500 mt-0.5">
              Allow the agent to autonomously remember user facts and preferences across chats.
            </p>
          </div>
          <Switch
            checked={memoryEnabled}
            onChange={setMemoryEnabled}
          />
        </div>

        <div className="flex items-center gap-3">
          <Button onClick={handleSaveSettings} disabled={isSaving} className="text-xs px-4">
            {isSaving ? "Saving..." : "Save Settings"}
          </Button>
          {saveSuccess && (
            <span className="flex items-center gap-1 text-xs font-medium text-green-500">
              <CheckIcon className="h-4 w-4" /> Settings saved successfully
            </span>
          )}
          {saveError && (
            <span className="text-xs font-medium text-red-500">{saveError}</span>
          )}
        </div>
      </div>

      {/* Memory Sandbox Block */}
      <div className="flex flex-col gap-4">
        <div>
          <h2 className="text-xs font-semibold text-neutral-700 dark:text-neutral-300">
            Memory Sandbox ({memories.length})
          </h2>
          <p className="text-xs text-neutral-500 mt-0.5">
            Review, edit, or delete persistent memories learned by the agent during conversations.
          </p>
        </div>

        <div className="flex flex-col gap-3">
          {memories.length === 0 ? (
            <div className="p-8 border border-dashed border-neutral-200 dark:border-neutral-800 rounded-xl text-xs text-neutral-500 text-center">
              No memories saved yet. Talk to the agent in chat to let it learn about you.
            </div>
          ) : (
            memories.map((m) => (
              <div
                key={m.id}
                className="p-4 bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-800 rounded-xl shadow-xs"
              >
                {editingMemory?.id === m.id ? (
                  <form onSubmit={handleUpdateMemory} className="flex flex-col gap-3">
                    <Input
                      placeholder="Title"
                      value={editingMemory.title || ""}
                      onChange={(e) =>
                        setEditingMemory({ ...editingMemory, title: e.target.value })
                      }
                    />
                    <textarea
                      className="w-full h-24 p-3 rounded-xl border border-neutral-200 bg-neutral-50 dark:border-neutral-800 dark:bg-neutral-950 text-neutral-900 dark:text-neutral-100 text-xs font-sans shadow-xs focus:outline-2 outline-blue-500 resize-y"
                      placeholder="Content"
                      value={editingMemory.content}
                      onChange={(e) =>
                        setEditingMemory({ ...editingMemory, content: e.target.value })
                      }
                    />
                    <div className="flex gap-2 justify-end">
                      <Button
                        type="button"
                        plain
                        onClick={() => setEditingMemory(null)}
                      >
                        Cancel
                      </Button>
                      <Button type="submit" className="text-xs">Save</Button>
                    </div>
                  </form>
                ) : (
                  <div>
                    <h3 className="text-xs font-semibold text-neutral-900 dark:text-neutral-100">
                      {m.title || "Untitled Memory"}
                    </h3>
                    <p className="text-xs text-neutral-600 dark:text-neutral-400 whitespace-pre-wrap mt-1">
                      {m.content}
                    </p>
                    <div className="flex justify-end gap-2 mt-3">
                      <Button
                        plain
                        className="py-1 px-2.5 text-xs"
                        onClick={() => setEditingMemory(m)}
                      >
                        Edit
                      </Button>
                      <Button
                        plain
                        className="py-1 px-2.5 text-xs text-red-600 dark:text-red-400 hover:text-red-700"
                        onClick={() => handleDeleteMemory(m.id)}
                      >
                        Delete
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

