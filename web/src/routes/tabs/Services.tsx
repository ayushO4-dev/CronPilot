import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, isApiError } from "../../lib/api";
import type { ServiceDetail, ServiceFile, ServiceUnit } from "../../lib/types";
import { Button, Input, Loading, StatusDot } from "../../components/ui";
import type { Status } from "../../components/ui";
import { Modal } from "../../components/Modal";
import { bytes } from "../../lib/format";
import styles from "./Services.module.css";

function statusOf(active: string): Status {
  switch (active) {
    case "active":
      return "ok";
    case "failed":
      return "err";
    case "activating":
    case "deactivating":
    case "reloading":
      return "warn";
    default:
      return "muted";
  }
}

type Filter =
  | "all"
  | "running"
  | "exited"
  | "dead"
  | "failed"
  | "enabled"
  | "disabled";
type SortKey = "name" | "activeState" | "subState" | "enabled";
type SortDir = "asc" | "desc";

const FILTERS: Filter[] = [
  "all",
  "running",
  "exited",
  "dead",
  "failed",
  "enabled",
  "disabled",
];

export function Services() {
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [sortKey, setSortKey] = useState<SortKey>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  const [selected, setSelected] = useState<string | null>(null);

  const {
    data: units,
    isLoading,
    isError,
  } = useQuery({
    queryKey: ["services"],
    queryFn: () => api.get<ServiceUnit[]>("/api/services"),
    refetchInterval: 5000,
  });

  if (isLoading) return <Loading text="reading services" />;
  if (isError)
    return <div className={styles.error}>failed to load services</div>;

  const needle = q.toLowerCase();
  const rows = (units ?? [])
    .filter((u) => {
      if (
        needle &&
        !u.name.toLowerCase().includes(needle) &&
        !u.description.toLowerCase().includes(needle)
      ) {
        return false;
      }
      if (filter === "failed")
        return u.activeState === "failed" || u.subState === "failed";
      if (filter === "enabled") return u.enabled === "enabled";
      if (filter === "disabled") return u.enabled === "disabled";
      if (filter !== "all") return u.subState === filter;
      return true;
    })
    .sort((a, b) => {
      const r = String(a[sortKey]).localeCompare(String(b[sortKey]));
      return sortDir === "asc" ? r : -r;
    });

  function setSort(key: SortKey) {
    if (key === sortKey) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("asc");
    }
  }
  const arrow = (key: SortKey) =>
    key === sortKey ? (sortDir === "asc" ? " ▲" : " ▼") : "";

  return (
    <div className={styles.page}>
      <div className={styles.toolbar}>
        <Input
          placeholder="filter services…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <div className={styles.filters}>
          {FILTERS.map((f) => (
            <Button
              key={f}
              small
              variant={filter === f ? "primary" : "default"}
              onClick={() => setFilter(f)}
            >
              {f}
            </Button>
          ))}
        </div>
        <span className={styles.count}>{rows.length} units</span>
      </div>

      <div className={styles.tableWrap}>
        <table>
          <thead>
            <tr>
              <th></th>
              <th className={styles.sortable} onClick={() => setSort("name")}>
                Unit{arrow("name")}
              </th>
              <th
                className={styles.sortable}
                onClick={() => setSort("activeState")}
              >
                Active{arrow("activeState")}
              </th>
              <th
                className={styles.sortable}
                onClick={() => setSort("subState")}
              >
                Sub{arrow("subState")}
              </th>
              <th
                className={styles.sortable}
                onClick={() => setSort("enabled")}
              >
                Enabled{arrow("enabled")}
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((u) => (
              <tr key={u.name} onClick={() => setSelected(u.name)}>
                <td>
                  <StatusDot
                    status={statusOf(u.activeState)}
                    title={u.activeState}
                  />
                </td>
                <td className={styles.unitCell}>
                  <div className={styles.unitName}>{u.name}</div>
                  <div className={styles.desc}>{u.description}</div>
                </td>
                <td>{u.activeState}</td>
                <td className={styles.muted}>{u.subState}</td>
                <td className={styles.muted}>{u.enabled || "—"}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td colSpan={5} className={styles.muted}>
                  no matching units
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {selected && (
        <ServiceModal name={selected} onClose={() => setSelected(null)} />
      )}
    </div>
  );
}

function ServiceModal({
  name,
  onClose,
}: {
  name: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [err, setErr] = useState("");
  const [editing, setEditing] = useState(false); // editor mounted
  const [panelOpen, setPanelOpen] = useState(false); // editor visible (animation)
  const openEditor = () => {
    setEditing(true);
    requestAnimationFrame(() => setPanelOpen(true));
  };
  const closeEditor = () => {
    setPanelOpen(false);
    window.setTimeout(() => setEditing(false), 220); // unmount after the exit anim
  };

  const { data: detail } = useQuery({
    queryKey: ["service", name],
    queryFn: () =>
      api.get<ServiceDetail>(`/api/services/${encodeURIComponent(name)}`),
    refetchInterval: 4000,
  });
  const { data: logs } = useQuery({
    queryKey: ["service-logs", name],
    queryFn: () =>
      api.get<{ lines: string[] | null }>(
        `/api/services/${encodeURIComponent(name)}/logs?lines=200`,
      ),
    refetchInterval: 5000,
  });

  const action = useMutation({
    mutationFn: (act: string) =>
      api.post(`/api/services/${encodeURIComponent(name)}/${act}`),
    onSuccess: () => {
      setErr("");
      qc.invalidateQueries({ queryKey: ["services"] });
      qc.invalidateQueries({ queryKey: ["service", name] });
      qc.invalidateQueries({ queryKey: ["service-logs", name] });
    },
    onError: (e) => setErr(isApiError(e) ? e.error : "action failed"),
  });

  const enabled = detail?.enabled === "enabled";
  const isActive = detail?.activeState === "active";

  const footer = (
    <>
      <Button
        small
        disabled={action.isPending || isActive}
        onClick={() => action.mutate("start")}
      >
        start
      </Button>
      <Button
        small
        variant="danger"
        disabled={action.isPending || !isActive}
        onClick={() => action.mutate("stop")}
      >
        stop
      </Button>
      <Button
        small
        disabled={action.isPending || !isActive}
        onClick={() => action.mutate("restart")}
      >
        restart
      </Button>
      {enabled ? (
        <Button
          small
          variant="danger"
          disabled={action.isPending}
          onClick={() => action.mutate("disable")}
        >
          disable
        </Button>
      ) : (
        <Button
          small
          disabled={action.isPending}
          onClick={() => action.mutate("enable")}
        >
          enable
        </Button>
      )}
    </>
  );

  return (
    <Modal
      title={name}
      onClose={onClose}
      actions={footer}
      width={panelOpen ? 520 : undefined}
      rightPanel={
        editing ? (
          <UnitFileEditor name={name} onDone={closeEditor} />
        ) : undefined
      }
      rightPanelOpen={panelOpen}
    >
      {err && (
        <div className={styles.error}>
          <span>{err}</span>
          <button
            className={styles.dismiss}
            onClick={() => setErr("")}
            aria-label="dismiss"
          >
            ✕
          </button>
        </div>
      )}

      {detail ? (
        <>
          <div className={styles.statusRow}>
            <StatusDot status={statusOf(detail.activeState)} />
            <span>
              {detail.activeState} ({detail.subState})
            </span>
          </div>
          <table className={styles.detailTable}>
            <tbody>
              <tr>
                <th>Description</th>
                <td>{detail.description || "—"}</td>
              </tr>
              <tr>
                <th>Load</th>
                <td>{detail.loadState}</td>
              </tr>
              <tr>
                <th>Enabled</th>
                <td>{detail.enabled || "—"}</td>
              </tr>
              <tr>
                <th>Main PID</th>
                <td>{detail.mainPID || "—"}</td>
              </tr>
              <tr>
                <th>Memory</th>
                <td>
                  {detail.memoryCurrent ? bytes(detail.memoryCurrent) : "—"}
                </td>
              </tr>
              <tr>
                <th>Since</th>
                <td>{detail.since || "—"}</td>
              </tr>
              <tr>
                <th>Unit file</th>
                <td>
                  <div className={styles.pathRow}>
                    <span className={styles.path}>
                      {detail.fragmentPath || "—"}
                    </span>
                    {detail.fragmentPath && !editing && (
                      <button className={styles.editBtn} onClick={openEditor}>
                        edit
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </>
      ) : (
        <Loading text="loading" />
      )}

      <div className={styles.logsHead}>Recent logs</div>
      <pre className={styles.logs}>
        {logs?.lines && logs.lines.length > 0
          ? logs.lines.join("\n")
          : "no log entries"}
      </pre>
    </Modal>
  );
}

// UnitFileEditor is the floating editor shown beside the service detail. It loads
// the unit's on-disk file and edits it in a textarea. Saving is gated behind
// authorization: if the daemon can write the file directly it's authorized
// immediately, otherwise the user unlocks it with the root password ("Sudo"),
// after which saves are written as root.
function UnitFileEditor({
  name,
  onDone,
}: {
  name: string;
  onDone: () => void;
}) {
  const qc = useQueryClient();
  const [content, setContent] = useState<string | null>(null);
  const [err, setErr] = useState("");
  const [authorized, setAuthorized] = useState(false);
  const [password, setPassword] = useState(""); // root password (sudo path only)
  const [showSudo, setShowSudo] = useState(false);
  const [sudoPw, setSudoPw] = useState("");

  const { data } = useQuery({
    queryKey: ["service-file", name],
    queryFn: () =>
      api.get<ServiceFile>(`/api/services/${encodeURIComponent(name)}/file`),
  });
  useEffect(() => {
    if (data && content === null) setContent(data.content);
    if (data?.writable) setAuthorized(true); // daemon can write directly
  }, [data, content]);

  const sudoCheck = useMutation({
    mutationFn: (pw: string) =>
      api.post(`/api/services/${encodeURIComponent(name)}/sudo-check`, {
        password: pw,
      }),
    onSuccess: (_d, pw) => {
      setPassword(pw);
      setAuthorized(true);
      setShowSudo(false);
      setSudoPw("");
      setErr("");
    },
    onError: (e) => setErr(isApiError(e) ? e.error : "authentication failed"),
  });

  const save = useMutation({
    mutationFn: ({ reload, body }: { reload: boolean; body: string }) =>
      api.put(`/api/services/${encodeURIComponent(name)}/file`, {
        content: body,
        reload,
        password, // "" when writing directly
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["service", name] });
      qc.invalidateQueries({ queryKey: ["service-file", name] });
      onDone();
    },
    onError: (e) => setErr(isApiError(e) ? e.error : "save failed"),
  });

  return (
    <>
      <header className={styles.editorHead}>
        <span className={styles.editorTitle}>{data?.path ?? "unit file"}</span>
        <button
          className={styles.dismiss}
          onClick={onDone}
          aria-label="close editor"
        >
          ✕
        </button>
      </header>
      {data && !data.writable && !authorized && (
        <div className={styles.readonlyNote}>
          the daemon can’t write this file — use <strong>Sudo</strong> to save as
          root
        </div>
      )}
      {err && (
        <div className={styles.error}>
          <span>{err}</span>
          <button
            className={styles.dismiss}
            onClick={() => setErr("")}
            aria-label="dismiss"
          >
            ✕
          </button>
        </div>
      )}
      <textarea
        className={styles.editorArea}
        value={content ?? ""}
        spellCheck={false}
        placeholder={data ? "" : "loading…"}
        onChange={(e) => setContent(e.target.value)}
      />
      <footer className={styles.editorFoot}>
        {!authorized && !showSudo && (
          <Button
            small
            onClick={() => {
              setShowSudo(true);
              setErr("");
            }}
          >
            Sudo
          </Button>
        )}
        {showSudo && (
          <div className={styles.sudoBox}>
            <Input
              type="password"
              placeholder="root password"
              value={sudoPw}
              autoFocus
              onChange={(e) => setSudoPw(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && sudoPw) sudoCheck.mutate(sudoPw);
              }}
            />
            <Button
              small
              variant="primary"
              disabled={!sudoPw || sudoCheck.isPending}
              onClick={() => sudoCheck.mutate(sudoPw)}
            >
              {sudoCheck.isPending ? "…" : "Unlock"}
            </Button>
            <button
              className={styles.dismiss}
              onClick={() => {
                setShowSudo(false);
                setSudoPw("");
                setErr("");
              }}
              aria-label="cancel sudo"
            >
              ✕
            </button>
          </div>
        )}
        {authorized && password !== "" && (
          <span className={styles.sudoOk}>🔓 root</span>
        )}
        <span style={{ flex: 1 }} />
        {authorized && (
          <SaveSplit
            pending={save.isPending}
            disabled={content === null}
            onSave={(reload) =>
              content !== null && save.mutate({ reload, body: content })
            }
          />
        )}
      </footer>
    </>
  );
}

// SaveSplit is a primary "Save ▾" button that opens a menu with the two save
// modes: plain save, or save followed by `systemctl daemon-reload`.
function SaveSplit({
  pending,
  disabled,
  onSave,
}: {
  pending: boolean;
  disabled: boolean;
  onSave: (reload: boolean) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className={styles.saveSplit}>
      {open && (
        <div className={styles.menuBackdrop} onClick={() => setOpen(false)} />
      )}
      <Button
        small
        variant="primary"
        disabled={pending || disabled}
        onClick={() => setOpen((o) => !o)}
      >
        {pending ? "saving…" : "Save ▾"}
      </Button>
      {open && (
        <div className={styles.saveMenu}>
          <button
            onClick={() => {
              setOpen(false);
              onSave(false);
            }}
          >
            Save
          </button>
          <button
            onClick={() => {
              setOpen(false);
              onSave(true);
            }}
          >
            Save and daemon-reload
          </button>
        </div>
      )}
    </div>
  );
}
