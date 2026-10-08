import { useState, type FormEvent } from 'react';
import type { HistoryEntry, Profile } from '../types';
import type { UseProfilesReturn } from '../hooks/useProfiles';
import { useSshConfigImport } from '../hooks/useSshConfigImport';
import { defaultFields, fieldsFromProfile, fieldsFromHistory, validateForm, type FormFields } from '../utils/form';
import { AuthFields } from './AuthFields';
import { JumpSection } from './JumpSection';
import './ConnectForm.css';
import { ProfilePicker, ENVIRONMENTS } from './ProfilePicker';

export interface ConnectionFormProps {
  store: UseProfilesReturn;
  history: HistoryEntry[];
  onSubmit: (entries: FormFields[]) => void;
  initialFields?: FormFields[];
}

export function ConnectionForm({ store, history, onSubmit, initialFields }: ConnectionFormProps) {
  const [entries, setEntries] = useState<FormFields[]>(() => initialFields?.length ? initialFields : [defaultFields()]);
  const [error, setError] = useState('');
  const [selectedProfile, setSelectedProfile] = useState<string | null>(null);
  const [profileName, setProfileName] = useState('');
  const [tag, setTag] = useState('');
  const [color, setColor] = useState('#7aa2f7');
  const [rememberKeys, setRememberKeys] = useState(false);
  const importer = useSshConfigImport(store.importProfiles);
  function patch(index: number, changes: Partial<FormFields>) {
    setEntries((prev) => prev.map((entry, i) => i === index ? { ...entry, ...changes } : entry));
    setError('');
  }
  function select(profile: Profile, index = 0) {
    patch(index, fieldsFromProfile(profile));
    if (index === 0) {
      setSelectedProfile(profile.id); setProfileName(profile.name); setTag(profile.tag ?? '');
      setColor(profile.color ?? '#7aa2f7'); setRememberKeys(!!profile.rememberKeys);
    }
  }
  function rememberSelectedKeys() {
    if (!selectedProfile) return;
    store.updateProfile(selectedProfile, { tag, color, rememberKeys });
    if (rememberKeys) store.storeProfileKeys(selectedProfile, {
      privateKeyContent: entries[0].privateKey, privateKeyName: entries[0].privateKeyName,
      jumpPrivateKeyContent: entries[0].jumpPrivateKey, jumpPrivateKeyName: entries[0].jumpPrivateKeyName,
    });
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    const filled = entries.filter((entry) => entry.host.trim());
    if (!filled.length) { setError('接続先を入力してください。'); return; }
    for (const [index, entry] of filled.entries()) {
      const invalid = validateForm(entry);
      if (invalid) { setError(`${entry.host || `接続先 ${index + 1}`}: ${invalid}`); return; }
    }
    rememberSelectedKeys();
    setError(''); onSubmit(filled.map((entry) => ({ ...entry })));
  }
  function quickConnect(profile: Profile) {
    const fields = fieldsFromProfile(profile);
    const invalid = validateForm(fields);
    if (invalid) { select(profile); setError(`この接続先に必要な認証情報を入力してください。 ${invalid}`); return; }
    onSubmit([fields]);
  }
  function saveProfile() {
    const entry = entries[0];
    const port = Number(entry.port);
    if (!entry.host.trim() || !entry.user.trim() || !Number.isInteger(port) || port < 1 || port > 65535) { setError('保存する接続先・ユーザー・ポートを入力してください。'); return; }
    store.saveProfile(profileName.trim() || `${entry.user}@${entry.host}`, entry.host.trim(), port, entry.user.trim(), entry.authType,
      entry.jumpHost ? { jumpHost: entry.jumpHost, jumpPort: Number(entry.jumpPort), jumpUser: entry.jumpUser, jumpAuthType: entry.jumpAuthType } : undefined,
      { privateKeyContent: rememberKeys ? entry.privateKey : undefined, privateKeyName: entry.privateKeyName,
        jumpPrivateKeyContent: rememberKeys ? entry.jumpPrivateKey : undefined, jumpPrivateKeyName: entry.jumpPrivateKeyName }, { tag, color, rememberKeys });
    setError('プロファイルを保存しました。');
  }
  return <div className="ux-connection-form">
    <ProfilePicker store={store} onSelect={select} onConnect={quickConnect} />
    {!!history.length && <section><h3>最近使った接続先</h3><div className="ux-chips">{history.map((entry) => <button key={`${entry.user}@${entry.host}:${entry.port}`} onClick={() => {
      const profile = store.profiles.find((p) => p.host === entry.host && p.port === entry.port && p.user === entry.user);
      if (profile) { select(profile); patch(0, { authType: entry.authType }); }
      else { setSelectedProfile(null); setRememberKeys(false); patch(0, fieldsFromHistory(entry)); }
    }}>{entry.user}@{entry.host} <small>{entry.authType}</small></button>)}</div></section>}
    <form noValidate onSubmit={submit}>
      {entries.map((entry, index) => <fieldset key={index} className="ux-host-fields">
        <legend>接続先 {index + 1}</legend>
        {index > 0 && <div className="ux-row"><select aria-label={`接続先 ${index + 1}のプロファイル`} value="" onChange={(e) => {
          const profile = store.profiles.find((p) => p.id === e.target.value); if (profile) select(profile, index);
        }}><option value="">プロファイルから選ぶ</option>{store.profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select><button type="button" onClick={() => setEntries((prev) => prev.filter((_, i) => i !== index))}>削除</button></div>}
        <div className="ux-row"><label className="ux-grow">ホスト<input autoFocus={index === 0} required value={entry.host} placeholder="hostname / 192.168.1.1" onChange={(e) => patch(index, { host: e.target.value })} /></label>
          <label className="ux-port">ポート<input type="number" min="1" max="65535" required value={entry.port} onChange={(e) => patch(index, { port: e.target.value })} /></label>
          <label>ユーザー<input required value={entry.user} autoComplete="username" onChange={(e) => patch(index, { user: e.target.value })} /></label></div>
        <AuthFields entry={entry} disabled={false} idPrefix={`connect-${index}`} onAuthTypeChange={(authType) => patch(index, { authType })}
          onFieldChange={(field, value) => patch(index, { [field]: value })} onKeyFile={(privateKey, privateKeyName) => patch(index, { privateKey, privateKeyName })} />
        <JumpSection entry={entry} disabled={false} idPrefix={`jump-${index}`} onFieldChange={(field, value) => patch(index, { [field]: value })}
          onClearJump={() => patch(index, { jumpHost: '', jumpPassword: '', jumpPrivateKey: '', jumpPassphrase: '' })}
          onJumpKeyFile={(jumpPrivateKey, jumpPrivateKeyName) => patch(index, { jumpPrivateKey, jumpPrivateKeyName })} />
      </fieldset>)}
      <div className="ux-row"><button type="button" disabled={entries.length >= 4} onClick={() => setEntries((prev) => [...prev, defaultFields()])}>＋ 接続先を追加</button>
        <button className="ux-primary" type="submit">{entries.length > 1 ? `${entries.length} 台に接続` : '接続する'}</button></div>
    </form>
    {error && <p role="status" className="ux-feedback">{error}</p>}
    <details className="ux-save-profile"><summary>接続先を保存・設定する</summary><div className="ux-row">
      <label>表示名<input value={profileName} onChange={(e) => setProfileName(e.target.value)} /></label>
      <label>環境タグ<select value={tag} onChange={(e) => setTag(e.target.value)}><option value="">なし</option>{ENVIRONMENTS.map((env) => <option key={env}>{env}</option>)}</select></label>
      <label>識別色<input type="color" value={color} onChange={(e) => setColor(e.target.value)} /></label></div>
      <label className="ux-inline"><input type="checkbox" checked={rememberKeys} onChange={(e) => setRememberKeys(e.target.checked)} />このブラウザタブで秘密鍵を記憶する</label>
      <p className="ux-muted">接続先の設定はこのブラウザに保存されます。記憶を選んだ秘密鍵は暗号化され、このタブを閉じると復号できなくなります。パスワード・パスフレーズは保存しません。</p>
      <div className="ux-row"><button onClick={saveProfile}>新しいプロファイルとして保存</button>{selectedProfile && <button onClick={rememberSelectedKeys}>選択中のプロファイル設定を更新</button>}</div>
    </details>
    <div className="ux-row"><button onClick={importer.openFilePicker}>SSH config を読み込む</button><button disabled={!importer.sshConfigFile} onClick={importer.reload}>再読込</button>
      <input ref={importer.fileInputRef} type="file" hidden onChange={importer.onFileChange} /></div>
    {importer.importMessage && <p role="status">{importer.importMessage}</p>}
  </div>;
}
