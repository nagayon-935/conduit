import { useRef } from 'react';
import type { AuthType } from '../types';
import type { FormFields } from '../utils/form';
import { parseKeyInfo } from '../utils/form';
import { KeyDropZone } from './KeyDropZone';
import { readFileText } from '../utils/readFileText';

const AUTH_TYPES: AuthType[] = ['vault', 'pubkey', 'password'];
const AUTH_LABELS: Record<AuthType, string> = { vault: 'Vault', pubkey: 'Public Key', password: 'Password' };

interface JumpSectionProps {
  entry: FormFields;
  disabled: boolean;
  idPrefix: string;
  onFieldChange: (field: keyof FormFields, value: string) => void;
  onClearJump: () => void;
  /** Called with the jump private key contents when a file is selected or dropped. */
  onJumpKeyFile: (content: string, fileName: string) => void;
}

/** Optional ProxyJump (bastion) host configuration with its own auth method. */
export function JumpSection({ entry, disabled, idPrefix, onFieldChange, onClearJump, onJumpKeyFile }: JumpSectionProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const hasJump = entry.jumpHost.trim() !== '';

  async function handleSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    onJumpKeyFile(await readFileText(file), file.name);
    e.target.value = '';
  }

  return (
    <div className="cf-jump-inline">
      <div className="cf-field">
        <label htmlFor={`${idPrefix}-jump-host`}>
          Jump Host <span className="cf-jump-optional">(ProxyJump — optional)</span>
        </label>
        <div className="cf-jump-host-row">
          <input
            id={`${idPrefix}-jump-host`}
            type="text"
            placeholder="jumphost.example.com"
            value={entry.jumpHost}
            onChange={(e) => onFieldChange('jumpHost', e.target.value)}
            disabled={disabled}
            autoComplete="off"
          />
          {hasJump && (
            <button
              type="button"
              className="cf-jump-clear-btn"
              onClick={onClearJump}
              disabled={disabled}
              title="Clear ProxyJump"
            >✕</button>
          )}
        </div>
      </div>

      <div className="cf-row">
        <div className="cf-field cf-field--port">
          <label htmlFor={`${idPrefix}-jump-port`}>Port</label>
          <input
            id={`${idPrefix}-jump-port`}
            type="number"
            placeholder="22"
            value={entry.jumpPort}
            onChange={(e) => onFieldChange('jumpPort', e.target.value)}
            disabled={disabled}
            min={1} max={65535}
          />
        </div>
        <div className="cf-field">
          <label htmlFor={`${idPrefix}-jump-user`}>User</label>
          <input
            id={`${idPrefix}-jump-user`}
            type="text"
            placeholder="ubuntu"
            value={entry.jumpUser}
            onChange={(e) => onFieldChange('jumpUser', e.target.value)}
            disabled={disabled}
            autoComplete="username"
          />
        </div>
      </div>

      {hasJump && (
        <div className="cf-jump-expanded">
          <div className="cf-auth-tabs">
            {AUTH_TYPES.map((at) => (
              <button
                key={at}
                type="button"
                className={`cf-auth-tab${entry.jumpAuthType === at ? ' active' : ''}`}
                onClick={() => onFieldChange('jumpAuthType', at)}
                disabled={disabled}
              >
                {AUTH_LABELS[at]}
              </button>
            ))}
          </div>

          {entry.jumpAuthType === 'password' && (
            <div className="cf-field">
              <label htmlFor={`${idPrefix}-jump-password`}>Password</label>
              <input
                id={`${idPrefix}-jump-password`}
                type="password"
                value={entry.jumpPassword}
                onChange={(e) => onFieldChange('jumpPassword', e.target.value)}
                disabled={disabled}
                autoComplete="current-password"
              />
            </div>
          )}

          {entry.jumpAuthType === 'pubkey' && (
            <div className="cf-field">
              <input ref={fileInputRef} type="file" style={{ display: 'none' }} onChange={handleSelect} />
              <KeyDropZone
                keyName={entry.jumpPrivateKeyName}
                keyLoaded={!!entry.jumpPrivateKey}
                disabled={disabled}
                onSelectClick={() => fileInputRef.current?.click()}
                onFileDrop={onJumpKeyFile}
              />
            </div>
          )}

          {entry.jumpAuthType === 'pubkey' && parseKeyInfo(entry.jumpPrivateKey)?.hasPassphrase && (
            <div className="cf-field">
              <label htmlFor={`${idPrefix}-jump-passphrase`}>Passphrase</label>
              <input
                id={`${idPrefix}-jump-passphrase`}
                type="password"
                value={entry.jumpPassphrase}
                onChange={(e) => onFieldChange('jumpPassphrase', e.target.value)}
                disabled={disabled}
                autoComplete="off"
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
