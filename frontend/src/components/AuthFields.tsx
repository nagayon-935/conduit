import { useRef } from 'react';
import type { AuthType } from '../types';
import type { FormFields, KeyInfo } from '../utils/form';
import { KeyDropZone } from './KeyDropZone';
import { readFileText } from '../utils/readFileText';

const AUTH_TYPES: AuthType[] = ['vault', 'pubkey', 'password'];
const AUTH_LABELS: Record<AuthType, string> = { vault: 'Vault', pubkey: 'Public Key', password: 'Password' };

interface AuthFieldsProps {
  entry: FormFields;
  disabled: boolean;
  idPrefix: string;
  /** Key validation badge (main entry only). */
  keyInfo?: KeyInfo | null;
  onAuthTypeChange: (at: AuthType) => void;
  onFieldChange: (field: keyof FormFields, value: string) => void;
  /** Called with the private key contents when a file is selected or dropped. */
  onKeyFile: (content: string, fileName: string) => void;
}

/** Auth method tabs (Vault / Public Key / Password) plus the matching inputs. */
export function AuthFields({ entry, disabled, idPrefix, keyInfo, onAuthTypeChange, onFieldChange, onKeyFile }: AuthFieldsProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);

  async function handleSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    onKeyFile(await readFileText(file), file.name);
    e.target.value = '';
  }

  async function handleDrop(file: File) {
    const content = await readFileText(file);
    if (!content.includes('-----BEGIN')) return;
    onKeyFile(content, file.name);
  }

  return (
    <>
      <div className="cf-auth-tabs">
        {AUTH_TYPES.map((at) => (
          <button
            key={at}
            type="button"
            className={`cf-auth-tab${entry.authType === at ? ' active' : ''}`}
            onClick={() => onAuthTypeChange(at)}
            disabled={disabled}
          >
            {AUTH_LABELS[at]}
          </button>
        ))}
      </div>

      {entry.authType === 'password' && (
        <div className="cf-field">
          <label htmlFor={`${idPrefix}-password`}>Password</label>
          <input
            id={`${idPrefix}-password`}
            name="password"
            type="password"
            value={entry.password}
            onChange={(e) => onFieldChange('password', e.target.value)}
            disabled={disabled}
            autoComplete="current-password"
          />
        </div>
      )}

      {entry.authType === 'pubkey' && (
        <div className="cf-field">
          <input ref={fileInputRef} type="file" style={{ display: 'none' }} onChange={handleSelect} />
          <KeyDropZone
            keyName={entry.privateKeyName}
            keyLoaded={!!entry.privateKey}
            keyInfo={keyInfo}
            disabled={disabled}
            onSelectClick={() => fileInputRef.current?.click()}
            onFileDrop={handleDrop}
          />
        </div>
      )}
    </>
  );
}
