import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Link } from "@/components/ui/link";
import { TrashIcon, PencilIcon, ArrowPathIcon, ChatBubbleLeftEllipsisIcon } from "@heroicons/react/24/outline";

export interface ScheduledTask {
  id: string;
  prompt: string;
  model: string;
  scheduled_at: string;
  status: "pending" | "running" | "completed" | "failed";
  created_at: string;
  updated_at: string;
  last_error?: string;
  chat_id?: string;
}

export function ScheduledTasks() {
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  const [editingTask, setEditingTask] = useState<ScheduledTask | null>(null);
  const [editPrompt, setEditPrompt] = useState("");
  const [editScheduledAt, setEditScheduledAt] = useState("");
  const [updating, setUpdating] = useState(false);

  const fetchTasks = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await fetch("/api/v1/scheduled");
      if (!res.ok) {
        throw new Error("Failed to fetch scheduled tasks");
      }
      const data = await res.json();
      setTasks(data.tasks || []);
    } catch (err: any) {
      setError(err.message || "An error occurred");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTasks();
    const interval = setInterval(fetchTasks, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleDelete = async (id: string) => {
    if (!confirm("Are you sure you want to delete this scheduled task?")) return;
    try {
      const res = await fetch(`/api/v1/scheduled/${id}`, { method: "DELETE" });
      if (!res.ok) throw new Error("Failed to delete task");
      setTasks((prev) => prev.filter((t) => t.id !== id));
    } catch (err: any) {
      alert(err.message || "Delete failed");
    }
  };

  const handleStartEdit = (task: ScheduledTask) => {
    setEditingTask(task);
    setEditPrompt(task.prompt);
    // Format date string for datetime-local input
    const d = new Date(task.scheduled_at);
    const localIso = new Date(d.getTime() - d.getTimezoneOffset() * 60000)
      .toISOString()
      .slice(0, 16);
    setEditScheduledAt(localIso);
  };

  const handleSaveEdit = async () => {
    if (!editingTask) return;
    try {
      setUpdating(true);
      const scheduledDate = new Date(editScheduledAt).toISOString();
      const res = await fetch(`/api/v1/scheduled/${editingTask.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          prompt: editPrompt,
          scheduled_at: scheduledDate,
        }),
      });
      if (!res.ok) throw new Error("Failed to update task");
      await fetchTasks();
      setEditingTask(null);
    } catch (err: any) {
      alert(err.message || "Update failed");
    } finally {
      setUpdating(false);
    }
  };

  const getStatusBadge = (status: ScheduledTask["status"]) => {
    switch (status) {
      case "pending":
        return <Badge color="zinc">Pending</Badge>;
      case "running":
        return <Badge color="indigo">Running</Badge>;
      case "completed":
        return <Badge color="green">Completed</Badge>;
      case "failed":
        return <Badge color="red">Failed</Badge>;
      default:
        return <Badge color="zinc">{status}</Badge>;
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-white text-neutral-900 dark:bg-neutral-900 dark:text-neutral-100 overflow-y-auto p-6">
      {/* Action Header */}
      <div className="flex shrink-0 items-center justify-between gap-3 pb-6">
        <div className="flex items-center gap-2">
          <span className="text-xs text-neutral-500 dark:text-neutral-400 font-medium">
            Scheduled Tasks ({tasks.length})
          </span>
        </div>
        <Button onClick={fetchTasks} color="white" className="px-3 text-xs">
          <ArrowPathIcon data-slot="icon" />
          Refresh
        </Button>
      </div>

      {loading && tasks.length === 0 ? (
        <div className="py-12 text-center text-sm text-neutral-500">Loading scheduled tasks...</div>
      ) : error ? (
        <div className="py-6 text-center text-sm text-red-500">{error}</div>
      ) : tasks.length === 0 ? (
        <div className="py-12 text-center border border-dashed border-neutral-200 dark:border-neutral-800 rounded-xl p-8">
          <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100">No scheduled tasks yet</p>
          <p className="text-xs text-neutral-500 mt-1">
            Ask the AI model in any chat to schedule a task (e.g., "Schedule a summary every morning at 9 AM").
          </p>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          {tasks.map((task) => (
            <div
              key={task.id}
              className="flex flex-col sm:flex-row items-start sm:items-center justify-between p-4 gap-4 bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-800 rounded-xl shadow-xs"
            >
              <div className="flex flex-col gap-1 min-w-0 flex-1">
                <div className="flex items-center gap-2 flex-wrap">
                  {getStatusBadge(task.status)}
                  <span className="text-xs font-mono text-neutral-500">
                    {new Date(task.scheduled_at).toLocaleString()}
                  </span>
                  <Badge color="zinc">{task.model}</Badge>
                </div>
                <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 mt-1 line-clamp-2">
                  {task.prompt}
                </p>
                {task.last_error && (
                  <p className="text-xs text-red-500 mt-1">Error: {task.last_error}</p>
                )}
              </div>

              <div className="flex items-center gap-2 shrink-0">
                {task.chat_id && (
                  <Link
                    to="/c/$chatId"
                    params={{ chatId: task.chat_id }}
                    className="inline-flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 font-medium"
                  >
                    <ChatBubbleLeftEllipsisIcon className="h-4 w-4" />
                    Open Chat
                  </Link>
                )}
                {task.status === "pending" && (
                  <Button onClick={() => handleStartEdit(task)} plain className="p-2">
                    <PencilIcon className="h-4 w-4" />
                  </Button>
                )}
                <Button onClick={() => handleDelete(task.id)} plain className="p-2 text-red-600 dark:text-red-400 hover:text-red-700">
                  <TrashIcon className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Edit Modal */}
      {editingTask && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-xs flex items-center justify-center p-4 z-50">
          <div className="bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-800 rounded-xl max-w-md w-full p-6 shadow-sm flex flex-col gap-4">
            <h3 className="text-base font-semibold text-neutral-900 dark:text-neutral-100">
              Edit Scheduled Task
            </h3>

            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-neutral-700 dark:text-neutral-300">
                Prompt
              </label>
              <Input
                value={editPrompt}
                onChange={(e) => setEditPrompt(e.target.value)}
                placeholder="Scheduled prompt"
              />
            </div>

            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium text-neutral-700 dark:text-neutral-300">
                Scheduled Time (Local)
              </label>
              <Input
                type="datetime-local"
                value={editScheduledAt}
                onChange={(e) => setEditScheduledAt(e.target.value)}
              />
            </div>

            <div className="flex justify-end gap-2 mt-4">
              <Button plain onClick={() => setEditingTask(null)} disabled={updating}>
                Cancel
              </Button>
              <Button onClick={handleSaveEdit} disabled={updating || !editPrompt || !editScheduledAt}>
                {updating ? "Saving..." : "Save Changes"}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

