import { useState, useCallback, useEffect } from 'react';
import type { Profile, AuthType, ProfileMetadata } from '../types';
import { readJSON, writeJSON } from '../utils/storage';
import { STORAGE_KEYS, MAX_PROFILES } from '../constants';
import { encryptText, decryptText } from '../utils/crypto';

const ENCRYPTED_PREFIX = 'enc:';

function isEncrypted(value: string): boolean {
  return value.startsWith(ENCRYPTED_PREFIX);
}

function isPlainKey(value: string): boolean {
  return value.includes('-----BEGIN');
}

async function encryptKeyContent(value: string): Promise<string> {
  if (!value) return value;
  if (isEncrypted(value)) return value; // already encrypted
  const cipher = await encryptText(value);
  return ENCRYPTED_PREFIX + cipher;
}

async function encryptProfileKeys(p: Profile): Promise<Profile> {
  const result = { ...p };
  if (!p.rememberKeys) {
    delete result.privateKeyContent; delete result.jumpPrivateKeyContent;
    return result;
  }
  if (p.privateKeyContent) {
    result.privateKeyContent = await encryptKeyContent(p.privateKeyContent);
  }
  if (p.jumpPrivateKeyContent) {
    result.jumpPrivateKeyContent = await encryptKeyContent(p.jumpPrivateKeyContent);
  }
  return result;
}

function loadFromStorage(): Profile[] {
  const raw = readJSON<Profile[]>(STORAGE_KEYS.PROFILES, []);
  // Default authType to 'vault' for old entries that lack it
  // Strip key content from in-memory state initially; decryption happens async
  return raw.map((p) => ({
    ...p,
    authType: p.authType ?? 'vault',
    rememberKeys: p.rememberKeys ?? !!(p.privateKeyContent || p.jumpPrivateKeyContent),
    privateKeyContent: p.privateKeyContent ? '' : undefined,
    jumpPrivateKeyContent: p.jumpPrivateKeyContent ? '' : undefined,
  }));
}

let saveRevision = 0;
async function saveToStorage(profiles: Profile[]): Promise<void> {
  const revision = ++saveRevision;
  const encrypted = await Promise.all(profiles.map(encryptProfileKeys));
  if (revision === saveRevision) writeJSON(STORAGE_KEYS.PROFILES, encrypted);
}

function generateId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}

interface JumpParams {
  jumpHost?: string;
  jumpPort?: number;
  jumpUser?: string;
  jumpAuthType?: AuthType;
}

interface KeyParams {
  privateKeyContent?: string;
  privateKeyName?: string;
  jumpPrivateKeyContent?: string;
  jumpPrivateKeyName?: string;
}

export interface ImportEntry {
  name: string; host: string; port: number; user: string;
  authType?: AuthType;
  identityFile?: string;
  jumpHost?: string; jumpPort?: number; jumpUser?: string;
  jumpIdentityFile?: string;
  localForwards?: { localPort: number; remoteHost: string; remotePort: number }[];
}

export type ImportProfilesFn = (entries: ImportEntry[], upsert?: boolean) => { added: number; updated: number };

export interface UseProfilesReturn {
  profiles: Profile[];
  saveProfile: (name: string, host: string, port: number, user: string, authType: AuthType, jump?: JumpParams, keys?: KeyParams, metadata?: ProfileMetadata) => void;
  updateProfile: (id: string, patch: ProfileMetadata & { authType?: AuthType }) => void;
  deleteProfile: (id: string) => void;
  loadProfile: (id: string) => Profile | undefined;
  storeProfileKeys: (id: string, keys: KeyParams) => void;
  importProfiles: ImportProfilesFn;
}

export function useProfiles(): UseProfilesReturn {
  const [profiles, setProfiles] = useState<Profile[]>(loadFromStorage);

  // Async-decrypt key content from localStorage on mount (or after profile changes)
  useEffect(() => {
    const raw = readJSON<Profile[]>(STORAGE_KEYS.PROFILES, []);
    const profilesWithKeys = raw.filter(
      (p) => (p.privateKeyContent && isEncrypted(p.privateKeyContent)) ||
             (p.jumpPrivateKeyContent && isEncrypted(p.jumpPrivateKeyContent)) ||
             (p.privateKeyContent && isPlainKey(p.privateKeyContent)) ||
             (p.jumpPrivateKeyContent && isPlainKey(p.jumpPrivateKeyContent))
    );
    if (profilesWithKeys.length === 0) return;

    let cancelled = false;
    (async () => {
      const decrypted = await Promise.all(
        profilesWithKeys.map(async (p) => {
          let privateKeyContent = p.privateKeyContent ?? '';
          let jumpPrivateKeyContent = p.jumpPrivateKeyContent ?? '';

          if (privateKeyContent) {
            if (isEncrypted(privateKeyContent)) {
              const plain = await decryptText(privateKeyContent.slice(ENCRYPTED_PREFIX.length));
              if (plain !== null) {
                privateKeyContent = plain;
              } else {
                // Stale key (tab was closed); clear content
                privateKeyContent = '';
              }
            }
            // plaintext — will be re-saved encrypted on next write
          }
          if (jumpPrivateKeyContent) {
            if (isEncrypted(jumpPrivateKeyContent)) {
              const plain = await decryptText(jumpPrivateKeyContent.slice(ENCRYPTED_PREFIX.length));
              if (plain !== null) {
                jumpPrivateKeyContent = plain;
              } else {
                jumpPrivateKeyContent = '';
              }
            }
          }
          return { id: p.id, privateKeyContent, jumpPrivateKeyContent };
        })
      );

      if (cancelled) return;
      setProfiles((prev) => {
        // まず復号済みの内容をプロファイルに反映
        const withDecrypted = prev.map((p) => {
          const d = decrypted.find((x) => x.id === p.id);
          if (!d || !p.rememberKeys) return p;
          return {
            ...p,
            ...(!p.privateKeyContent && d.privateKeyContent ? { privateKeyContent: d.privateKeyContent } : {}),
            ...(!p.jumpPrivateKeyContent && d.jumpPrivateKeyContent ? { jumpPrivateKeyContent: d.jumpPrivateKeyContent } : {}),
          };
        });
        return withDecrypted;
      });
    })();
    return () => { cancelled = true; };
    // Run once on mount only
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveProfile = useCallback((name: string, host: string, port: number, user: string, authType: AuthType, jump?: JumpParams, keys?: KeyParams, metadata: ProfileMetadata = {}) => {
    const newProfile: Profile = {
      id: generateId(),
      name,
      host,
      port,
      user,
      authType,
      createdAt: new Date().toISOString(),
      ...metadata,
      privateKeyName: keys?.privateKeyName,
      jumpPrivateKeyName: keys?.jumpPrivateKeyName,
      ...(metadata.rememberKeys && keys?.privateKeyContent ? { privateKeyContent: keys.privateKeyContent, privateKeyName: keys.privateKeyName } : {}),
      ...(jump?.jumpHost ? {
        jumpHost: jump.jumpHost,
        jumpPort: jump.jumpPort,
        jumpUser: jump.jumpUser,
        jumpAuthType: jump.jumpAuthType,
        ...(metadata.rememberKeys && keys?.jumpPrivateKeyContent ? { jumpPrivateKeyContent: keys.jumpPrivateKeyContent, jumpPrivateKeyName: keys.jumpPrivateKeyName } : {}),
      } : {}),
    };
    setProfiles((prev) => {
      const updated = [newProfile, ...prev].slice(0, MAX_PROFILES);
      void saveToStorage(updated).catch(() => {}); // fire-and-forget async encrypt+save
      return updated;
    });
  }, []);

  const deleteProfile = useCallback((id: string) => {
    setProfiles((prev) => {
      const updated = prev.filter((p) => p.id !== id);
      void saveToStorage(updated).catch(() => {});
      return updated;
    });
  }, []);

  const loadProfile = useCallback(
    (id: string): Profile | undefined => profiles.find((p) => p.id === id),
    [profiles],
  );

  const updateProfile = useCallback((id: string, patch: ProfileMetadata & { authType?: AuthType }) => {
    setProfiles((prev) => {
      const updated = prev.map((p) => {
        if (p.id !== id) return p;
        const next = { ...p, ...patch };
        if (patch.rememberKeys === false) { delete next.privateKeyContent; delete next.jumpPrivateKeyContent; }
        return next;
      });
      void saveToStorage(updated).catch(() => {});
      return updated;
    });
  }, []);

  const storeProfileKeys = useCallback((id: string, keys: KeyParams) => {
    setProfiles((prev) => {
      const updated = prev.map((p) => p.id === id && p.rememberKeys ? { ...p, ...keys } : p);
      void saveToStorage(updated).catch(() => {});
      return updated;
    });
  }, []);

  // upsert=false: 重複をスキップして追加のみ
  // upsert=true : 既存プロファイルを上書き更新し、新規は追加（鍵内容は保持）
  const importProfiles = useCallback(
    (entries: ImportEntry[], upsert = false): { added: number; updated: number } => {
      let added = 0;
      let updated = 0;
      setProfiles((prev) => {
        const existingMap = new Map(prev.map((p) => [`${p.name}|${p.host}`, p]));
        const next = upsert
          ? prev.map((p) => {
              const incoming = entries.find((e) => `${e.name}|${e.host}` === `${p.name}|${p.host}`);
              if (!incoming) return p;
              updated++;
              // IdentityFile がある場合は pubkey を優先; 保存済み鍵内容は引き継ぐ
              const resolvedAuthType = incoming.identityFile
                ? 'pubkey'
                : (incoming.authType ?? (p.authType !== 'vault' ? p.authType : 'vault'));
              const keyBasename = incoming.identityFile
                ? (incoming.identityFile.split('/').pop()?.split('\\').pop() ?? incoming.identityFile)
                : undefined;
              const jumpKeyBasename = incoming.jumpIdentityFile
                ? (incoming.jumpIdentityFile.split('/').pop()?.split('\\').pop() ?? incoming.jumpIdentityFile)
                : undefined;
              const resolvedJumpAuthType: AuthType | undefined = incoming.jumpIdentityFile
                ? 'pubkey'
                : (incoming.jumpHost ? (p.jumpAuthType ?? 'vault') : undefined);
              // 接続情報を更新しつつ保存済みの鍵内容は引き継ぐ
              return {
                ...p,
                host: incoming.host,
                port: incoming.port,
                user: incoming.user,
                authType: resolvedAuthType,
                // 鍵内容が未保存かつ IdentityFile 情報があればファイル名だけ更新
                ...(keyBasename && !p.privateKeyContent ? { privateKeyName: keyBasename } : {}),
                ...(incoming.jumpHost ? {
                  jumpHost: incoming.jumpHost,
                  jumpPort: incoming.jumpPort,
                  jumpUser: incoming.jumpUser,
                  jumpAuthType: resolvedJumpAuthType,
                  ...(jumpKeyBasename && !p.jumpPrivateKeyContent ? { jumpPrivateKeyName: jumpKeyBasename } : {}),
                } : {}),
                ...(incoming.localForwards ? { localForwards: incoming.localForwards } : {}),
              };
            })
          : [...prev];
        entries
          .filter((e) => !existingMap.has(`${e.name}|${e.host}`))
          .forEach((e) => {
            const resolvedAuthType: AuthType = e.identityFile ? 'pubkey' : (e.authType ?? 'vault');
            const keyBasename = e.identityFile
              ? (e.identityFile.split('/').pop()?.split('\\').pop() ?? e.identityFile)
              : undefined;
            const jumpKeyBasename = e.jumpIdentityFile
              ? (e.jumpIdentityFile.split('/').pop()?.split('\\').pop() ?? e.jumpIdentityFile)
              : undefined;
            const resolvedJumpAuthType: AuthType = e.jumpIdentityFile ? 'pubkey' : 'vault';
            next.unshift({
              id: generateId(),
              name: e.name,
              host: e.host,
              port: e.port,
              user: e.user,
              authType: resolvedAuthType,
              createdAt: new Date().toISOString(),
              ...(keyBasename ? { privateKeyName: keyBasename } : {}),
              ...(e.jumpHost ? {
                jumpHost: e.jumpHost,
                jumpPort: e.jumpPort,
                jumpUser: e.jumpUser,
                jumpAuthType: resolvedJumpAuthType,
                ...(jumpKeyBasename ? { jumpPrivateKeyName: jumpKeyBasename } : {}),
              } : {}),
              ...(e.localForwards ? { localForwards: e.localForwards } : {}),
            });
            added++;
          });
        const result = next.slice(0, MAX_PROFILES);
        void saveToStorage(result).catch(() => {});
        return result;
      });
      return { added, updated };
    },
    [],
  );

  return { profiles, saveProfile, updateProfile, deleteProfile, loadProfile, storeProfileKeys, importProfiles };
}
