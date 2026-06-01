import { useState, useRef, type FormEvent } from 'react';
import { connectToHost } from '../api/connect';
import type { AppState, AuthType, HistoryEntry } from '../types';
import { useProfiles } from '../hooks/useProfiles';
import { useSshConfigImport } from '../hooks/useSshConfigImport';
import { useNavMenu } from '../hooks/useNavMenu';
import { ConnectTopBar } from './ConnectTopBar';
import { AuthFields } from './AuthFields';
import { JumpSection } from './JumpSection';
import { KeyRequiredModal, type PendingKey } from './KeyRequiredModal';
import { type FormFields, defaultFields, validateForm, buildConnectRequest, matchProfile, parseKeyInfo, type KeyInfo } from '../utils/form';
import './ConnectForm.css';

interface ConnectFormProps {
  appState: AppState;
  onConnect: (sessionToken: string, expiresAt: string, host: string, port: number, user: string, authType: AuthType) => void;
  onStateChange: (state: AppState) => void;
  history?: HistoryEntry[];
  onShowSessions?: () => void;
  onShowLogs?: () => void;
  sessionCount?: number;
}

const FEATURES = [
  { icon: '⚡', text: 'Short-lived certs' },
  { icon: '🔄', text: 'Grace period' },
  { icon: '🖥️', text: 'Multi-tab sharing' },
  { icon: '🔐', text: 'In-memory keys' },
  { icon: '🌐', text: 'Auto-reconnect' },
];

export function ConnectForm({
  appState,
  onConnect,
  onStateChange,
  history = [],
  onShowSessions,
  onShowLogs,
  sessionCount,
}: ConnectFormProps) {
  const [fields, setFields] = useState<FormFields>(defaultFields);
  const [extraEntries, setExtraEntries] = useState<FormFields[]>([]);
  const [error, setError] = useState<string | null>(null);
  const isLoading = appState === 'connecting';

  const { profiles, saveProfile, deleteProfile, importProfiles, storeProfileKeys } = useProfiles();
  const [showSaveProfile, setShowSaveProfile] = useState(false);
  const [profileName, setProfileName] = useState('');

  const { sshConfigFile, importMessage, fileInputRef, openFilePicker, reload, onFileChange } =
    useSshConfigImport(importProfiles);
  const nav = useNavMenu();

  const [loadedProfileId, setLoadedProfileId] = useState<string | null>(null);
  const [keyModalPending, setKeyModalPending] = useState<PendingKey[] | null>(null);
  // Key validation feedback (main entry only)
  const [mainKeyInfo, setMainKeyInfo] = useState<KeyInfo | null>(null);
  // Host autocomplete
  const [showHostSuggestions, setShowHostSuggestions] = useState(false);
  const hostInputRef = useRef<HTMLInputElement>(null);
  const profilesRef = useRef(profiles);
  profilesRef.current = profiles;
  const loadedProfileIdRef = useRef(loadedProfileId);
  loadedProfileIdRef.current = loadedProfileId;

  // Apply a loaded private key to the main entry (+ persist it to the loaded profile).
  function applyMainKey(content: string, fileName: string) {
    setFields((prev) => ({ ...prev, privateKey: content, privateKeyName: fileName }));
    setMainKeyInfo(parseKeyInfo(content));
    if (loadedProfileIdRef.current) {
      storeProfileKeys(loadedProfileIdRef.current, { privateKeyContent: content, privateKeyName: fileName });
    }
  }

  function applyMainJumpKey(content: string, fileName: string) {
    setFields((prev) => ({ ...prev, jumpPrivateKey: content, jumpPrivateKeyName: fileName }));
    if (loadedProfileIdRef.current) {
      storeProfileKeys(loadedProfileIdRef.current, { jumpPrivateKeyContent: content, jumpPrivateKeyName: fileName });
    }
  }

  function handleHistoryClick(entry: HistoryEntry) {
    setLoadedProfileId(null);
    setFields({ host: entry.host, port: String(entry.port), user: entry.user, authType: entry.authType ?? 'vault', password: '', privateKey: '', privateKeyName: '', jumpHost: '', jumpPort: '22', jumpUser: '', jumpAuthType: 'vault', jumpPassword: '', jumpPrivateKey: '', jumpPrivateKeyName: '' });
    if (error) setError(null);
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) {
    const { name, value } = e.target;
    setFields((prev) => ({ ...prev, [name]: value }));
    if (error) setError(null);
  }

  function handleAuthTypeChange(authType: AuthType) {
    setFields((prev) => ({ ...prev, authType }));
    if (error) setError(null);
  }

  function handleExtraChange(index: number, field: keyof FormFields, value: string) {
    setExtraEntries((prev) => prev.map((entry, i) => i === index ? { ...entry, [field]: value } : entry));
    if (error) setError(null);
  }

  function handleExtraAuthTypeChange(index: number, authType: AuthType) {
    setExtraEntries((prev) => prev.map((entry, i) => i === index ? { ...entry, authType } : entry));
    if (error) setError(null);
  }

  function handleModalKeyFileChange(basename: string, keyType: 'main' | 'jump', e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const content = (ev.target?.result as string) ?? '';
      // Store into profile(s) that need this key
      for (const p of profilesRef.current) {
        if (keyType === 'main' && p.privateKeyName === basename && !p.privateKeyContent) {
          storeProfileKeys(p.id, { privateKeyContent: content, privateKeyName: file.name });
          if (p.id === loadedProfileIdRef.current) {
            setFields(prev => ({ ...prev, privateKey: content, privateKeyName: file.name }));
          }
        }
        if (keyType === 'jump' && p.jumpPrivateKeyName === basename && !p.jumpPrivateKeyContent) {
          storeProfileKeys(p.id, { jumpPrivateKeyContent: content, jumpPrivateKeyName: file.name });
          if (p.id === loadedProfileIdRef.current) {
            setFields(prev => ({ ...prev, jumpPrivateKey: content, jumpPrivateKeyName: file.name }));
          }
        }
      }
      // Also update fields directly if loaded profile
      if (keyType === 'main' && loadedProfileIdRef.current) {
        setFields(prev => ({ ...prev, privateKey: content, privateKeyName: file.name }));
      }
      // Check if all pending keys are now filled → auto proceed
      setKeyModalPending((prev) => {
        if (!prev) return null;
        const remaining = prev.filter((item) => item.basename !== basename);
        if (remaining.length === 0) {
          // All keys provided — dismiss modal and proceed
          setTimeout(() => {
            document.getElementById('cf-form')?.dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }));
          }, 50);
          return null;
        }
        return remaining;
      });
    };
    reader.readAsText(file);
    e.target.value = '';
  }

  function handleModalCancel() {
    setKeyModalPending(null);
  }


  function clearJumpFields(entryIndex: number | 'main') {
    const cleared = { jumpHost: '', jumpPort: '22', jumpUser: '', jumpAuthType: 'vault' as const, jumpPassword: '', jumpPrivateKey: '', jumpPrivateKeyName: '' };
    if (entryIndex === 'main') {
      setFields((prev) => ({ ...prev, ...cleared }));
    } else {
      setExtraEntries((prev) => prev.map((entry, i) => i === entryIndex ? { ...entry, ...cleared } : entry));
    }
  }

  function addExtraEntry() {
    setExtraEntries((prev) => [...prev, defaultFields()]);
  }

  function removeExtraEntry(index: number) {
    setExtraEntries((prev) => prev.filter((_, i) => i !== index));
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(null);

    const allEntries = [fields, ...extraEntries].filter((entry) => entry.host.trim());

    // Validate all entries
    for (const entry of allEntries) {
      const validationError = validateForm(entry);
      if (validationError) { setError(validationError); return; }
    }

    if (allEntries.length === 0) {
      setError('Host is required.');
      return;
    }

    // Check if any pubkey entry is missing its private key — show modal if so
    if (fields.authType === 'pubkey' && !fields.privateKey) {
      const pending: Array<{ basename: string; keyType: 'main' | 'jump' }> = [];
      const seen = new Set<string>();
      for (const p of profilesRef.current) {
        if (p.authType === 'pubkey' && p.privateKeyName && !p.privateKeyContent) {
          if (!seen.has(p.privateKeyName)) { seen.add(p.privateKeyName); pending.push({ basename: p.privateKeyName, keyType: 'main' }); }
        }
        if (p.jumpAuthType === 'pubkey' && p.jumpPrivateKeyName && !p.jumpPrivateKeyContent) {
          if (!seen.has(p.jumpPrivateKeyName!)) { seen.add(p.jumpPrivateKeyName!); pending.push({ basename: p.jumpPrivateKeyName!, keyType: 'jump' }); }
        }
      }
      if (pending.length > 0) {
        setKeyModalPending(pending);
        return; // Connection will be re-triggered from modal via handleModalProceed
      }
    }

    onStateChange('connecting');

    if (allEntries.length === 1) {
      // Single host: original behaviour
      const entry = allEntries[0];
      const port = parseInt(entry.port, 10);
      try {
        const connectReq = buildConnectRequest(entry);
        const response = await connectToHost(connectReq);
        onConnect(response.session_token, response.expires_at, entry.host.trim(), port, entry.user.trim(), entry.authType);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'An unexpected error occurred.');
        onStateChange('idle');
      }
      return;
    }

    // Multiple hosts: connect in parallel
    const results = await Promise.allSettled(
      allEntries.map((entry) => connectToHost(buildConnectRequest(entry)))
    );

    results.forEach((result, i) => {
      if (result.status === 'fulfilled') {
        const entry = allEntries[i];
        onConnect(result.value.session_token, result.value.expires_at, entry.host.trim(), parseInt(entry.port, 10), entry.user.trim(), entry.authType);
      }
    });

    const failedIndices = results.reduce<number[]>((acc, r, i) => {
      if (r.status === 'rejected') acc.push(i);
      return acc;
    }, []);
    if (failedIndices.length > 0 && failedIndices.length === results.length) {
      setError('All connections failed.');
      onStateChange('idle');
    } else if (failedIndices.length > 0) {
      const failedHosts = failedIndices.map((i) => allEntries[i].host);
      setError(`${results.length - failedIndices.length}/${results.length} connected. Failed: ${failedHosts.join(', ')}`);
      onStateChange('idle');
    } else {
      onStateChange('idle');
    }
  }

  function handleSaveProfile() {
    const validationError = validateForm(fields);
    if (validationError) { setError(validationError); return; }
    const name = profileName.trim() || `${fields.user}@${fields.host}`;
    const jh = fields.jumpHost.trim();
    saveProfile(
      name, fields.host.trim(), parseInt(fields.port, 10), fields.user.trim(), fields.authType,
      jh ? { jumpHost: jh, jumpPort: parseInt(fields.jumpPort, 10), jumpUser: fields.jumpUser.trim() || undefined, jumpAuthType: fields.jumpAuthType } : undefined,
      {
        privateKeyContent: fields.privateKey || undefined,
        privateKeyName: fields.privateKeyName || undefined,
        jumpPrivateKeyContent: fields.jumpPrivateKey || undefined,
        jumpPrivateKeyName: fields.jumpPrivateKeyName || undefined,
      },
    );
    setProfileName('');
    setShowSaveProfile(false);
  }

  function handleLoadProfile(id: string) {
    const p = profiles.find((x) => x.id === id);
    if (p) {
      setLoadedProfileId(id);
      setFields({
        host: p.host, port: String(p.port), user: p.user, authType: p.authType ?? 'vault',
        password: '',
        privateKey: p.privateKeyContent ?? '',
        privateKeyName: p.privateKeyName ?? '',
        jumpHost: p.jumpHost ?? '',
        jumpPort: p.jumpPort ? String(p.jumpPort) : '22',
        jumpUser: p.jumpUser ?? '',
        jumpAuthType: p.jumpAuthType ?? 'vault',
        jumpPassword: '',
        jumpPrivateKey: p.jumpPrivateKeyContent ?? '',
        jumpPrivateKeyName: p.jumpPrivateKeyName ?? '',
      });
      if (error) setError(null);
    }
  }

  function handleExtraLoadProfile(index: number, id: string) {
    const p = profiles.find((x) => x.id === id);
    if (p) {
      setExtraEntries((prev) =>
        prev.map((entry, i) =>
          i === index ? {
            host: p.host, port: String(p.port), user: p.user, authType: p.authType ?? 'vault',
            password: '',
            privateKey: p.privateKeyContent ?? '',
            privateKeyName: p.privateKeyName ?? '',
            jumpHost: p.jumpHost ?? '',
            jumpPort: p.jumpPort ? String(p.jumpPort) : '22',
            jumpUser: p.jumpUser ?? '',
            jumpAuthType: p.jumpAuthType ?? 'vault',
            jumpPassword: '',
            jumpPrivateKey: p.jumpPrivateKeyContent ?? '',
            jumpPrivateKeyName: p.jumpPrivateKeyName ?? '',
          } : entry
        )
      );
      if (error) setError(null);
    }
  }

  function handleExtraLoadHistory(index: number, h: HistoryEntry) {
    setExtraEntries((prev) =>
      prev.map((entry, i) =>
        i === index ? { host: h.host, port: String(h.port), user: h.user, authType: h.authType ?? 'vault', password: '', privateKey: '', privateKeyName: '', jumpHost: '', jumpPort: '22', jumpUser: '', jumpAuthType: 'vault', jumpPassword: '', jumpPrivateKey: '', jumpPrivateKeyName: '' } : entry
      )
    );
    if (error) setError(null);
  }

  const hasMultiple = extraEntries.length > 0;

  return (
    <div className="cf-page">
      <ConnectTopBar
        nav={nav}
        onShowSessions={onShowSessions}
        onShowLogs={onShowLogs}
        sessionCount={sessionCount}
      />

      <div className="cf-container">
        {/* Form card */}
        <div
          className="cf-card"
        >
          <form id="cf-form" className="cf-form" onSubmit={handleSubmit} noValidate>
            <div className="cf-field">
              <label htmlFor="host">Host</label>
              <div className="cf-host-ac-wrap">
                <input
                  ref={hostInputRef}
                  id="host" name="host" type="text"
                  placeholder="192.168.1.1 or hostname.example.com"
                  value={fields.host}
                  onChange={(e) => {
                    handleChange(e);
                    setShowHostSuggestions(true);
                  }}
                  onFocus={() => { if (profiles.length > 0) setShowHostSuggestions(true); }}
                  onBlur={() => setTimeout(() => setShowHostSuggestions(false), 150)}
                  disabled={isLoading} autoComplete="off" autoFocus
                />
                {showHostSuggestions && (() => {
                  const q = fields.host.toLowerCase();
                  const suggestions = profiles
                    .filter(p =>
                      !q ||
                      p.host.toLowerCase().includes(q) ||
                      p.name.toLowerCase().includes(q) ||
                      p.user.toLowerCase().includes(q)
                    )
                    .slice(0, 8);
                  if (suggestions.length === 0) return null;
                  return (
                    <div className="cf-host-suggestions">
                      {suggestions.map((p) => (
                        <button
                          key={p.id}
                          type="button"
                          className="cf-host-suggestion-item"
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() => {
                            handleLoadProfile(p.id);
                            setShowHostSuggestions(false);
                          }}
                        >
                          <span className="cf-host-suggestion-name">{p.name}</span>
                          <span className="cf-host-suggestion-detail">
                            {p.user}@{p.host}{p.port !== 22 ? `:${p.port}` : ''}
                          </span>
                        </button>
                      ))}
                    </div>
                  );
                })()}
              </div>
            </div>

            <div className="cf-row">
              <div className="cf-field cf-field--port">
                <label htmlFor="port">Port</label>
                <input
                  id="port" name="port" type="number"
                  placeholder="22" value={fields.port}
                  onChange={handleChange} disabled={isLoading}
                  min={1} max={65535}
                />
              </div>
              <div className="cf-field">
                <label htmlFor="user">User</label>
                <input
                  id="user" name="user" type="text"
                  placeholder="ubuntu" value={fields.user}
                  onChange={handleChange} disabled={isLoading}
                  autoComplete="username"
                />
              </div>
            </div>

            <AuthFields
              entry={fields}
              disabled={isLoading}
              idPrefix="main"
              keyInfo={mainKeyInfo}
              onAuthTypeChange={handleAuthTypeChange}
              onFieldChange={(field, value) => setFields((prev) => ({ ...prev, [field]: value }))}
              onKeyFile={applyMainKey}
            />

            <JumpSection
              entry={fields}
              disabled={isLoading}
              idPrefix="main"
              onFieldChange={(field, value) => setFields((prev) => ({ ...prev, [field]: value }))}
              onClearJump={() => clearJumpFields('main')}
              onJumpKeyFile={applyMainJumpKey}
            />
          </form>

          {/* Extra host entries — same layout as main form */}
          {extraEntries.map((entry, i) => (
            <div key={i} className="cf-extra-card">
              <div className="cf-extra-card-header">
                <span className="cf-extra-card-label">Host {i + 2}</span>
                <button
                  type="button"
                  className="cf-extra-remove"
                  onClick={() => removeExtraEntry(i)}
                  disabled={isLoading}
                  title="Remove"
                >
                  ✕
                </button>
              </div>

              <div className="cf-field">
                <label>Host</label>
                <input
                  type="text"
                  placeholder="192.168.1.1 or hostname.example.com"
                  value={entry.host}
                  onChange={(e) => handleExtraChange(i, 'host', e.target.value)}
                  disabled={isLoading}
                  autoComplete="off"
                />
              </div>

              <div className="cf-row">
                <div className="cf-field cf-field--port">
                  <label>Port</label>
                  <input
                    type="number"
                    placeholder="22"
                    value={entry.port}
                    onChange={(e) => handleExtraChange(i, 'port', e.target.value)}
                    disabled={isLoading}
                    min={1}
                    max={65535}
                  />
                </div>
                <div className="cf-field">
                  <label>User</label>
                  <input
                    type="text"
                    placeholder="ubuntu"
                    value={entry.user}
                    onChange={(e) => handleExtraChange(i, 'user', e.target.value)}
                    disabled={isLoading}
                    autoComplete="username"
                  />
                </div>
              </div>

              <AuthFields
                entry={entry}
                disabled={isLoading}
                idPrefix={`extra-${i}`}
                onAuthTypeChange={(at) => handleExtraAuthTypeChange(i, at)}
                onFieldChange={(field, value) => handleExtraChange(i, field, value)}
                onKeyFile={(content, name) =>
                  setExtraEntries((prev) => prev.map((e2, j) =>
                    j === i ? { ...e2, privateKey: content, privateKeyName: name } : e2))
                }
              />

              <JumpSection
                entry={entry}
                disabled={isLoading}
                idPrefix={`extra-${i}`}
                onFieldChange={(field, value) => handleExtraChange(i, field, value)}
                onClearJump={() => clearJumpFields(i)}
                onJumpKeyFile={(content, name) =>
                  setExtraEntries((prev) => prev.map((e2, j) =>
                    j === i ? { ...e2, jumpPrivateKey: content, jumpPrivateKeyName: name } : e2))
                }
              />

              {/* Profile chips + Recent chips for this entry */}
              {(profiles.length > 0 || history.length > 0) && (
                <div className="cf-extra-chips">
                  {profiles.map((p) => (
                    <button
                      key={`p-${p.id}`}
                      type="button"
                      className="cf-extra-profile-chip"
                      onClick={() => handleExtraLoadProfile(i, p.id)}
                      disabled={isLoading}
                      title={`${p.host}:${p.port} · ${p.user}`}
                    >
                      {p.name}
                    </button>
                  ))}
                  {history.map((h, hi) => (
                    <button
                      key={`h-${hi}`}
                      type="button"
                      className="cf-extra-history-chip"
                      onClick={() => handleExtraLoadHistory(i, h)}
                      disabled={isLoading}
                      title={`${h.host}:${h.port} · ${h.user}`}
                    >
                      {(() => {
                        const matched = matchProfile(profiles, h.host, h.port, h.user);
                        return matched ? matched.name : `${h.user}@${h.host}${h.port !== 22 ? `:${h.port}` : ''}`;
                      })()}
                    </button>
                  ))}
                </div>
              )}
            </div>
          ))}

          {/* Add host button */}
          <div className="cf-add-host-row">
            <button
              type="button"
              className="cf-add-host-btn"
              onClick={addExtraEntry}
              disabled={isLoading || extraEntries.length >= 3}
              title={extraEntries.length >= 3 ? 'Maximum 4 hosts' : undefined}
            >
              + Add host
            </button>
            {extraEntries.length > 0 && (
              <span className="cf-host-count">{extraEntries.length + 1} / 4</span>
            )}
          </div>

          {/* Connect button — applies to all hosts */}
          <div className="cf-connect-row">
            <button
              type="submit"
              form="cf-form"
              className="cf-btn"
              disabled={isLoading}
            >
              {isLoading
                ? <><span className="cf-spinner" aria-hidden="true" />Connecting…</>
                : hasMultiple ? `Connect All (${extraEntries.length + 1})` : 'Connect'}
            </button>
          </div>

          {/* SSH config import (hidden file input) */}
          <input
            ref={fileInputRef}
            type="file"
            style={{ display: 'none' }}
            onChange={onFileChange}
          />

          {error && <div className="cf-error" role="alert">{error}</div>}
          {importMessage && <div className="cf-import-message" role="status">{importMessage}</div>}

          {/* Profiles section */}
          <div className="cf-profiles">
            <div className="cf-profiles-header">
              <p className="cf-profiles-label">Profiles</p>
              <div className="cf-profiles-actions">
                <button
                  type="button"
                  className="cf-save-profile-btn"
                  onClick={() => setShowSaveProfile((v) => !v)}
                  disabled={isLoading}
                >
                  + Save
                </button>
                <button
                  type="button"
                  className="cf-reload-btn"
                  onClick={reload}
                  disabled={isLoading || !sshConfigFile}
                  title={sshConfigFile ? `Reload: ${sshConfigFile.name}` : 'Import a config file first'}
                >
                  ↻ Reload
                </button>
                <button
                  type="button"
                  className="cf-import-btn"
                  onClick={openFilePicker}
                  disabled={isLoading}
                  title="Import hosts from ~/.ssh/config"
                >
                  Import ~/.ssh/config
                </button>
              </div>
            </div>
            {showSaveProfile && (
              <div className="cf-save-profile-inline">
                <input
                  type="text"
                  className="cf-profile-name-input"
                  placeholder={fields.user && fields.host ? `${fields.user}@${fields.host}` : 'Profile name'}
                  value={profileName}
                  onChange={(e) => setProfileName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleSaveProfile();
                    if (e.key === 'Escape') setShowSaveProfile(false);
                  }}
                  autoFocus
                />
                <button type="button" className="cf-save-profile-confirm" onClick={handleSaveProfile}>
                  Save
                </button>
                <button type="button" className="cf-save-profile-cancel" onClick={() => setShowSaveProfile(false)}>
                  ×
                </button>
              </div>
            )}
          {profiles.length > 0 && (
            <ul className="cf-profiles-list">
              {profiles.map((p) => (
                <li key={p.id} className="cf-profile-item">
                  <button
                    type="button"
                    className="cf-profile-load"
                    onClick={() => handleLoadProfile(p.id)}
                    disabled={isLoading}
                  >
                    <span className="cf-profile-name">{p.name}</span>
                    <span className="cf-profile-detail">{p.host}:{p.port} · {p.user}</span>
                  </button>
                  <button
                    type="button"
                    className="cf-profile-delete"
                    onClick={() => deleteProfile(p.id)}
                    title="Delete profile"
                    disabled={isLoading}
                  >
                    ✕
                  </button>
                </li>
              ))}
            </ul>
          )}
          </div>

          {history.length > 0 && (
            <div className="cf-history">
              <div className="cf-history-header">
                <p className="cf-history-label">Recent</p>
                {history.length > 5 && onShowLogs && (
                  <button type="button" className="cf-history-view-all" onClick={onShowLogs}>
                    View all ({history.length}) →
                  </button>
                )}
              </div>
              <ul className="cf-history-list">
                {history.slice(0, 5).map((entry, i) => (
                  <li
                    key={i} className="cf-history-item"
                    role="button" tabIndex={0}
                    onClick={() => handleHistoryClick(entry)}
                    onKeyDown={(e) => e.key === 'Enter' && handleHistoryClick(entry)}
                  >
                    {(() => {
                      const matched = matchProfile(profiles, entry.host, entry.port, entry.user);
                      return matched ? (
                        <>
                          <span className="cf-history-host">{matched.name}</span>
                          <span className="cf-history-user">{entry.host}:{entry.port} · {entry.user}</span>
                        </>
                      ) : (
                        <>
                          <span className="cf-history-host">{entry.host}:{entry.port}</span>
                          <span className="cf-history-user">as {entry.user}</span>
                        </>
                      );
                    })()}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>

        {/* Feature chips */}
        <ul className="cf-features">
          {FEATURES.map((f) => (
            <li key={f.text} className="cf-feature">
              <span>{f.icon}</span> {f.text}
            </li>
          ))}
        </ul>

      </div>

      {keyModalPending && (
        <KeyRequiredModal
          pending={keyModalPending}
          onCancel={handleModalCancel}
          onKeyFileChange={handleModalKeyFileChange}
        />
      )}
    </div>
  );
}
