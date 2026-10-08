import { useRef, useState } from 'react';
import type { AuthType } from '../types';
import type { FormFields, KeyInfo } from '../utils/form';
import { parseKeyInfo } from '../utils/form';
import { KeyDropZone } from './KeyDropZone';
import { readFileText } from '../utils/readFileText';

const AUTH_TYPES: AuthType[] = ['vault', 'pubkey', 'password'];
const AUTH_LABELS: Record<AuthType, string> = { vault: 'Vault', pubkey: '秘密鍵', password: 'パスワード' };
const AUTH_HELP = { vault: 'Vault から短命の SSH 証明書を自動取得します。接続先で Vault の認証が設定されている場合に利用できます。', pubkey: '接続先に登録済みの公開鍵と対になる秘密鍵を選択します。暗号化された鍵にはパスフレーズを入力します。', password: '接続先のアカウントのパスワードを入力します。パスワードは保存しません。' };

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
  const [fileError, setFileError] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  async function handleSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    try { onKeyFile(await readFileText(file), file.name); setFileError(''); }
    catch { setFileError('秘密鍵ファイルを読み込めませんでした。もう一度選択してください。'); }
    finally { e.target.value = ''; }
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

      <p className="ux-muted">{AUTH_HELP[entry.authType]}</p>
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
            keyInfo={keyInfo ?? parseKeyInfo(entry.privateKey)}
            disabled={disabled}
            onSelectClick={() => fileInputRef.current?.click()}
            onFileDrop={onKeyFile}
          />
        </div>
      )}

      {fileError && <p role="alert">{fileError}</p>}
      {entry.authType === 'pubkey' && parseKeyInfo(entry.privateKey)?.hasPassphrase && (
        <div className="cf-field">
          <label htmlFor={`${idPrefix}-passphrase`}>Passphrase</label>
          <input
            id={`${idPrefix}-passphrase`}
            name="passphrase"
            type="password"
            value={entry.passphrase}
            onChange={(e) => onFieldChange('passphrase', e.target.value)}
            disabled={disabled}
            autoComplete="off"
          />
        </div>
      )}
    </>
  );
}
