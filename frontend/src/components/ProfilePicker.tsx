import { useState } from 'react';
import type { Profile } from '../types';
import type { UseProfilesReturn } from '../hooks/useProfiles';

export const ENVIRONMENTS = ['本番', '検証', '開発'];
export function ProfilePicker({ store, onSelect, onConnect }: {
  store: UseProfilesReturn; onSelect: (profile: Profile) => void; onConnect: (profile: Profile) => void;
}) {
  const [query, setQuery] = useState('');
  const [favoritesOnly, setFavoritesOnly] = useState(false);
  const profiles = [...store.profiles].filter((p) => (!favoritesOnly || p.favorite)
    && `${p.name} ${p.host} ${p.user} ${p.tag ?? ''}`.toLowerCase().includes(query.toLowerCase()))
    .sort((a, b) => Number(!!b.favorite) - Number(!!a.favorite));
  return <section className="ux-profiles" aria-label="接続先プロファイル">
    <div className="ux-section-heading"><h3>接続先</h3><label className="ux-inline"><input type="checkbox" checked={favoritesOnly} onChange={(e) => setFavoritesOnly(e.target.checked)} />お気に入りのみ</label></div>
    <input aria-label="接続先を検索" placeholder="名前・ホスト・ユーザー・タグで検索" value={query} onChange={(e) => setQuery(e.target.value)} />
    <div className="ux-profile-list">
      {profiles.map((p) => <div key={p.id} className="ux-profile" style={{ borderLeftColor: p.color || '#7aa2f7' }}>
        <button className="ux-star" aria-label={`${p.name}のお気に入りを${p.favorite ? '解除' : '登録'}`} aria-pressed={!!p.favorite} onClick={() => store.updateProfile(p.id, { favorite: !p.favorite })}>{p.favorite ? '★' : '☆'}</button>
        <button className="ux-profile-main" onClick={() => onSelect(p)}><strong>{p.name}</strong><small>{p.user}@{p.host}:{p.port} · {p.authType}</small></button>
        <select aria-label={`${p.name}の環境タグ`} value={p.tag ?? ''} onChange={(e) => store.updateProfile(p.id, { tag: e.target.value })}><option value="">タグなし</option>{ENVIRONMENTS.map((tag) => <option key={tag}>{tag}</option>)}</select>
        <input type="color" aria-label={`${p.name}の色`} value={/^#[\da-f]{6}$/i.test(p.color ?? '') ? p.color : '#7aa2f7'} onChange={(e) => store.updateProfile(p.id, { color: e.target.value })} />
        <button onClick={() => onConnect(p)} aria-label={`${p.name}に接続`}>接続</button>
        <button className="ux-subtle" onClick={() => store.deleteProfile(p.id)} aria-label={`${p.name}を削除`}>×</button>
      </div>)}
      {profiles.length === 0 && <p className="ux-muted">{store.profiles.length ? '一致する接続先がありません。' : '接続先を保存すると、次回からここで選べます。'}</p>}
    </div>
  </section>;
}
