import { useState, useEffect, useRef, type FormEvent } from 'react';
import { connectToHost } from '../api/connect';
import type { HistoryEntry, Profile, AuthType } from '../types';
import { type FormFields, defaultFields, validateForm, buildConnectRequest, matchProfile, fieldsFromHistory, fieldsFromProfile, clearedJumpFields } from '../utils/form';
import { readFileText } from '../utils/readFileText';
import './NewConnectionOverlay.css';

interface NewConnectionOverlayProps {
  onConnect: (token: string, expiresAt: string, host: string, port: number, user: string, authType: AuthType) => void;
  onClose: () => void;
  history?: HistoryEntry[];
  profiles?: Profile[];
}

export function NewConnectionOverlay({
  onConnect,
  onClose,
  history = [],
  profiles = [],
}: NewConnectionOverlayProps) {
  const [fields, setFields] = useState<FormFields>(defaultFields);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const keyFileRef = useRef<HTMLInputElement>(null);
  const jumpKeyFileRef = useRef<HTMLInputElement>(null);
  // Close on Escape key
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && !isLoading) {
        onClose();
      }
    }
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose, isLoading]);

  function handleChange(e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) {
    const { name, value } = e.target;
    setFields((prev) => ({ ...prev, [name]: value }));
    if (error) setError(null);
  }

  function handleAuthTypeChange(authType: AuthType) {
    setFields((prev) => ({ ...prev, authType }));
    if (error) setError(null);
  }

  async function handleJumpKeyFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await readFileText(file);
    setFields((prev) => ({ ...prev, jumpPrivateKey: text, jumpPrivateKeyName: file.name }));
    e.target.value = '';
  }

  function clearJumpFields() {
    setFields((prev) => ({ ...prev, ...clearedJumpFields() }));
  }

  async function handleKeyFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await readFileText(file);
    setFields((prev) => ({ ...prev, privateKey: text, privateKeyName: file.name }));
    e.target.value = '';
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(null);
    const validationError = validateForm(fields);
    if (validationError) {
      setError(validationError);
      return;
    }
    const port = parseInt(fields.port, 10);
    setIsLoading(true);
    try {
      const connectReq = buildConnectRequest(fields);
      const response = await connectToHost(connectReq);
      onConnect(response.session_token, response.expires_at, fields.host.trim(), port, fields.user.trim(), fields.authType);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An unexpected error occurred.');
      setIsLoading(false);
    }
  }

  function fillFromHistory(entry: HistoryEntry) {
    if (isLoading) return;
    setFields(fieldsFromHistory(entry));
    if (error) setError(null);
  }

  function fillFromProfile(profile: Profile) {
    if (isLoading) return;
    setFields(fieldsFromProfile(profile));
    if (error) setError(null);
  }

  function handleBackdropClick(e: React.MouseEvent<HTMLDivElement>) {
    if (e.target === e.currentTarget && !isLoading) {
      onClose();
    }
  }

  return (
    <div className="nco-backdrop" onClick={handleBackdropClick} aria-modal="true" role="dialog">
      <div className="nco-card">
        <h2 className="nco-title">New Connection</h2>

        <button
          className="nco-close-btn"
          aria-label="Close overlay"
          onClick={onClose}
          disabled={isLoading}
        >
          ✕
        </button>

        <form className="nco-form" onSubmit={handleSubmit} noValidate>
          <div className="nco-field">
            <label htmlFor="nco-host">Host</label>
            <input
              id="nco-host"
              name="host"
              type="text"
              placeholder="192.168.1.1 or hostname.example.com"
              value={fields.host}
              onChange={handleChange}
              disabled={isLoading}
              autoComplete="off"
              autoFocus
            />
          </div>

          <div className="nco-row">
            <div className="nco-field nco-field--port">
              <label htmlFor="nco-port">Port</label>
              <input
                id="nco-port"
                name="port"
                type="number"
                placeholder="22"
                value={fields.port}
                onChange={handleChange}
                disabled={isLoading}
                min={1}
                max={65535}
              />
            </div>
            <div className="nco-field">
              <label htmlFor="nco-user">User</label>
              <input
                id="nco-user"
                name="user"
                type="text"
                placeholder="ubuntu"
                value={fields.user}
                onChange={handleChange}
                disabled={isLoading}
                autoComplete="username"
              />
            </div>
          </div>

          {/* Auth type selector */}
          <div className="nco-auth-tabs">
            {(['vault', 'password', 'pubkey'] as AuthType[]).map((at) => (
              <button
                key={at}
                type="button"
                className={`nco-auth-tab${fields.authType === at ? ' active' : ''}`}
                onClick={() => handleAuthTypeChange(at)}
                disabled={isLoading}
              >
                {at === 'vault' ? 'Vault' : at === 'password' ? 'Password' : 'Public Key'}
              </button>
            ))}
          </div>

          {fields.authType === 'password' && (
            <div className="nco-field">
              <label htmlFor="nco-password">Password</label>
              <input
                id="nco-password"
                name="password"
                type="password"
                placeholder="••••••••"
                value={fields.password}
                onChange={handleChange}
                disabled={isLoading}
                autoComplete="current-password"
              />
            </div>
          )}

          {fields.authType === 'pubkey' && (
            <div className="nco-field">
              <label>Private Key</label>
              <div className="nco-key-picker-row">
                <input
                  ref={keyFileRef}
                  type="file"
                  style={{ display: 'none' }}
                  onChange={handleKeyFileChange}
                />
                <button
                  type="button"
                  className="nco-key-upload-btn"
                  onClick={() => keyFileRef.current?.click()}
                  disabled={isLoading}
                >
                  Choose key file…
                </button>
                {fields.privateKeyName ? (
                  <span className="nco-key-filename" title={fields.privateKeyName}>
                    {fields.privateKeyName}
                  </span>
                ) : (
                  <span className="nco-key-placeholder">No file selected</span>
                )}
              </div>
            </div>
          )}

          {/* ProxyJump — always inline */}
          <div className="nco-jump-inline">
            <div className="nco-field">
              <label htmlFor="nco-jump-host">
                Jump Host <span className="nco-jump-optional">(ProxyJump — optional)</span>
              </label>
              <div className="nco-jump-host-row">
                <input
                  id="nco-jump-host"
                  type="text"
                  placeholder="jumphost.example.com"
                  value={fields.jumpHost}
                  onChange={(e) => setFields((prev) => ({ ...prev, jumpHost: e.target.value }))}
                  disabled={isLoading}
                  autoComplete="off"
                />
                {fields.jumpHost.trim() && (
                  <button
                    type="button"
                    className="nco-jump-clear-btn"
                    onClick={clearJumpFields}
                    disabled={isLoading}
                    title="Clear ProxyJump"
                  >✕</button>
                )}
              </div>
            </div>

            {fields.jumpHost.trim() && (
              <div className="nco-jump-expanded">
                <div className="nco-row">
                  <div className="nco-field nco-field--port">
                    <label htmlFor="nco-jump-port">Port</label>
                    <input
                      id="nco-jump-port"
                      type="number"
                      placeholder="22"
                      value={fields.jumpPort}
                      onChange={(e) => setFields((prev) => ({ ...prev, jumpPort: e.target.value }))}
                      disabled={isLoading}
                      min={1} max={65535}
                    />
                  </div>
                  <div className="nco-field">
                    <label htmlFor="nco-jump-user">User</label>
                    <input
                      id="nco-jump-user"
                      type="text"
                      placeholder="ubuntu"
                      value={fields.jumpUser}
                      onChange={(e) => setFields((prev) => ({ ...prev, jumpUser: e.target.value }))}
                      disabled={isLoading}
                      autoComplete="username"
                    />
                  </div>
                </div>

                <div className="nco-auth-tabs">
                  {(['vault', 'password', 'pubkey'] as AuthType[]).map((at) => (
                    <button
                      key={at}
                      type="button"
                      className={`nco-auth-tab${fields.jumpAuthType === at ? ' active' : ''}`}
                      onClick={() => setFields((prev) => ({ ...prev, jumpAuthType: at }))}
                      disabled={isLoading}
                    >
                      {at === 'vault' ? 'Vault' : at === 'password' ? 'Password' : 'Public Key'}
                    </button>
                  ))}
                </div>

                {fields.jumpAuthType === 'password' && (
                  <div className="nco-field">
                    <label htmlFor="nco-jump-password">Password</label>
                    <input
                      id="nco-jump-password"
                      type="password"
                      placeholder="••••••••"
                      value={fields.jumpPassword}
                      onChange={(e) => setFields((prev) => ({ ...prev, jumpPassword: e.target.value }))}
                      disabled={isLoading}
                      autoComplete="current-password"
                    />
                  </div>
                )}

                {fields.jumpAuthType === 'pubkey' && (
                  <div className="nco-field">
                    <label>Private Key</label>
                    <div className="nco-key-picker-row">
                      <input
                        ref={jumpKeyFileRef}
                        type="file"
                        style={{ display: 'none' }}
                        onChange={handleJumpKeyFileChange}
                      />
                      <button
                        type="button"
                        className="nco-key-upload-btn"
                        onClick={() => jumpKeyFileRef.current?.click()}
                        disabled={isLoading}
                      >
                        Choose key file…
                      </button>
                      {fields.jumpPrivateKeyName ? (
                        <span className="nco-key-filename" title={fields.jumpPrivateKeyName}>{fields.jumpPrivateKeyName}</span>
                      ) : (
                        <span className="nco-key-placeholder">No file selected</span>
                      )}
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>

          <button type="submit" className="nco-submit-btn" disabled={isLoading}>
            {isLoading ? (
              <>
                <span className="nco-spinner" aria-hidden="true" />
                Connecting…
              </>
            ) : (
              'Connect'
            )}
          </button>
        </form>

        {error && (
          <div className="nco-error" role="alert">
            {error}
          </div>
        )}

        {profiles.length > 0 && (
          <div className="nco-quick-section">
            <p className="nco-quick-label">Profiles</p>
            <div className="nco-chips">
              {profiles.map((p) => (
                <button
                  key={p.id}
                  className="nco-chip"
                  type="button"
                  disabled={isLoading}
                  onClick={() => fillFromProfile(p)}
                  title={`${p.host}:${p.port} · ${p.user}`}
                >
                  {p.name}
                </button>
              ))}
            </div>
          </div>
        )}

        {history.length > 0 && (
          <div className="nco-quick-section">
            <p className="nco-quick-label">Recent</p>
            <div className="nco-chips">
              {history.map((entry, i) => (
                <button
                  key={i}
                  className="nco-chip"
                  type="button"
                  disabled={isLoading}
                  onClick={() => fillFromHistory(entry)}
                  title={`${entry.host}:${entry.port} as ${entry.user}`}
                >
                  {(() => {
                    const matched = matchProfile(profiles, entry.host, entry.port, entry.user);
                    return matched ? matched.name : `${entry.user}@${entry.host}${entry.port !== 22 ? `:${entry.port}` : ''}`;
                  })()}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
