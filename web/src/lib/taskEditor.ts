// Persistent task-editor state. Lives outside the Tasks route so switching
// browser tabs (which unmounts the route) does not reset the selection, edit
// mode, or an in-progress draft.
import { create } from "zustand";
import type { Task } from "./types";

interface TaskEditorStore {
  selectedId: string | null;
  newTaskId: string | null;
  editMode: boolean;
  draft: Task | null;

  /** Select a task; selecting a different task clears any edit in progress. */
  select: (id: string | null) => void;
  setNewTaskId: (id: string | null) => void;
  /** Enter edit mode with a draft clone. */
  startEdit: (draft: Task) => void;
  /** Update the in-progress draft. */
  setDraft: (draft: Task) => void;
  /** Leave edit mode and discard the draft. */
  stopEdit: () => void;
}

export const useTaskEditor = create<TaskEditorStore>((set) => ({
  selectedId: null,
  newTaskId: null,
  editMode: false,
  draft: null,

  select: (id) =>
    set((s) =>
      s.selectedId === id ? s : { selectedId: id, editMode: false, draft: null },
    ),
  setNewTaskId: (id) => set({ newTaskId: id }),
  startEdit: (draft) => set({ editMode: true, draft }),
  setDraft: (draft) => set({ draft }),
  stopEdit: () => set({ editMode: false, draft: null }),
}));
