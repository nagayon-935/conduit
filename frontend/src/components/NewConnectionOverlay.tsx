import { useState, useEffect, useRef, type FormEvent } from 'react';
import { connectToHost } from '../api/connect';
import type { HistoryEntry, Profile, AuthType } from '../types';
import { type FormFields, type KeyInfo, defaultFields, validateForm, buildConnectRequest, matchProfile, parseKeyInfo, fieldsFromHistory, fieldsFromProfile, clearedJumpFields } from '../utils/form';
import type { UseProfilesReturn } from '../hooks/useProfiles';
import { useSshConfigImport } from '../hooks/useSshConfigImport';
import { AuthFields } from './AuthFields';
import { JumpSection } from './JumpSection';
import './NewConnectionOverlay.css';

interface NewConnectionOverlayProps {
  onConnect: (token: string, expiresAt: string, host: string, port: number, user: string, authType: AuthType) => void;
  onClose: () => void;
  history?: HistoryEntry[];
  profiles?: Profile[];
  saveProfile: UseProfilesReturn['saveProfile'];
  storeProfileKeys: UseProfilesReturn['storeProfileKeys'];
  importProfiles: UseProfilesReturn['importProfiles'];
  deleteProfile: UseProfilesReturn['deleteProfile'];
}

export function NewConnectionOverlay({
  onConnect,
  onClose,
  history = [],
  profiles = [],
  saveProfile,
  storeProfileKeys,
  importProfiles,
  deleteProfile,
}: NewConnectionOverlayProps) {
  const [fields, setFields] = useState<FormFields>(defaultFields);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [keyInfo, setKeyInfo] = useState<KeyInfo | null>(null);
  const [showSaveProfile, setShowSaveProfile] = useState(false);
  const [profileName, setProfileName] = useState('');
  const [loadedProfileId, setLoadedProfileId] = useState<string | null>(null);
  const { sshConfigFile, importMessage, fileInputRef, openFilePicker, reload, onFileChange } =
    useSshConfigImport(importProfiles);
  const loadedProfileIdRef = useRef(loadedProfileId);
  loadedProfileIdRef.current = loadedProfileId;
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

  function applyKey(content: string, fileName: string) {
    setFields((prev) => ({ ...prev, privateKey: content, privateKeyName: fileName }));
    setKeyInfo(parseKeyInfo(content));
    if (loadedProfileIdRef.current) {
      storeProfileKeys(loadedProfileIdRef.current, { privateKeyContent: content, privateKeyName: fileName });
    }
  }

  function applyJumpKey(content: string, fileName: string) {
    setFields((prev) => ({ ...prev, jumpPrivateKey: content, jumpPrivateKeyName: fileName }));
    if (loadedProfileIdRef.current) {
      storeProfileKeys(loadedProfileIdRef.current, { jumpPrivateKeyContent: content, jumpPrivateKeyName: fileName });
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

  function clearJumpFields() {
    setFields((prev) => ({ ...prev, ...clearedJumpFields() }));
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
    setLoadedProfileId(profile.id);
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

          <AuthFields
            entry={fields}
            disabled={isLoading}
            idPrefix="nco"
            keyInfo={keyInfo}
            onAuthTypeChange={handleAuthTypeChange}
            onFieldChange={(field, value) => setFields((prev) => ({ ...prev, [field]: value }))}
            onKeyFile={applyKey}
          />

          <JumpSection
            entry={fields}
            disabled={isLoading}
            idPrefix="nco"
            onFieldChange={(field, value) => setFields((prev) => ({ ...prev, [field]: value }))}
            onClearJump={clearJumpFields}
            onJumpKeyFile={applyJumpKey}
          />

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

        {/* SSH config import (hidden file input) */}
        <input
          ref={fileInputRef}
          type="file"
          style={{ display: 'none' }}
          onChange={onFileChange}
        />

        {error && (
          <div className="nco-error" role="alert">
            {error}
          </div>
        )}
        {importMessage && <div className="cf-import-message" role="status">{importMessage}</div>}

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
                    onClick={() => fillFromProfile(p)}
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
