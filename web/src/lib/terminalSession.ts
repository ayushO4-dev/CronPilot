// Persistent terminal-session state. Lives outside the Terminal route so the
// shell stays "open" across browser-tab switches: the server keeps the PTY
// alive, and on remount the view reconnects (without a ticket) and replays the
// session's recent output. The ticket is one-shot — used only for the initial
// connect that starts the shell.
import { create } from "zustand";

interface TerminalSessionStore {
  active: boolean;
  user: string | null;
  endedUser: string | null;
  ticket: string | null;

  start: (user: string, ticket: string) => void;
  end: () => void;
  /** Returns the pending one-shot ticket (and clears it). Null on reconnect. */
  consumeTicket: () => string | null;
}

export const useTerminalSession = create<TerminalSessionStore>((set, get) => ({
  active: false,
  user: null,
  endedUser: null,
  ticket: null,

  start: (user, ticket) => set({ active: true, user, ticket, endedUser: null }),
  end: () =>
    set((s) => ({ active: false, user: null, ticket: null, endedUser: s.user })),
  consumeTicket: () => {
    const t = get().ticket;
    if (t) set({ ticket: null });
    return t;
  },
}));
