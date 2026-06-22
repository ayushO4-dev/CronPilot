import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../lib/api";
import type { Settings as SettingsT } from "../../lib/types";
import { useAuth } from "../../lib/auth";
import { initialTheme, saveTheme } from "../../lib/theme";
import type { Theme } from "../../lib/theme";
import { Button, Panel } from "../../components/ui";
import { ChangePasswordForm } from "../../components/ChangePasswordForm";
import { TwoFactorPanel } from "../../components/TwoFactorPanel";
import { UpdatePanel } from "../../components/UpdatePanel";
import { duration } from "../../lib/format";
import styles from "./tabs.module.css";

export function Settings() {
  const { user, logout } = useAuth();
  const [theme, setTheme] = useState<Theme>(initialTheme());
  const { data } = useQuery({
    queryKey: ["settings"],
    queryFn: () => api.get<SettingsT>("/api/settings"),
  });

  function changeTheme(t: Theme) {
    setTheme(t);
    void saveTheme(t);
  }

  return (
    <div className={styles.page}>
      <div className={styles.settingsGrid}>
        {/* Account & security */}
        <div className={styles.settingsCol}>
          <Panel title="Change password">
            <ChangePasswordForm />
          </Panel>

          <Panel title="Two-factor authentication">
            <TwoFactorPanel />
          </Panel>
        </div>

        {/* System */}
        <div className={styles.settingsCol}>
          <Panel title="Appearance">
            <div className={styles.row}>
              <span>Theme</span>
              <div className={styles.themeBtns}>
                <Button
                  small
                  variant={theme === "dark" ? "primary" : "default"}
                  onClick={() => changeTheme("dark")}
                >
                  Dark
                </Button>
                <Button
                  small
                  variant={theme === "light" ? "primary" : "default"}
                  onClick={() => changeTheme("light")}
                >
                  Light
                </Button>
              </div>
            </div>
          </Panel>

          <Panel title="Software update">
            <UpdatePanel />
          </Panel>

          <Panel title="Server">
            <dl className={styles.kv}>
              <dt>Version</dt>
              <dd>{data?.version ?? "—"}</dd>
              <dt>Mode</dt>
              <dd>{data ? (data.dev ? "development" : "production") : "—"}</dd>
              <dt>Session idle timeout</dt>
              <dd>{data ? duration(data.sessionIdleSeconds) : "—"}</dd>
              <dt>Session max lifetime</dt>
              <dd>{data ? duration(data.sessionMaxSeconds) : "—"}</dd>
            </dl>
          </Panel>
        </div>
      </div>
    </div>
  );
}
