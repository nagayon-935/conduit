import { describe, it, expect } from 'vitest';
import { fieldsFromHistory, fieldsFromProfile, clearedJumpFields, buildConnectRequest, defaultFields } from './form';
import type { HistoryEntry, Profile } from '../types';

describe('fieldsFromHistory', () => {
  it('maps host/port/user/authType and leaves keys/jump empty', () => {
    const entry: HistoryEntry = { host: 'h', port: 2222, user: 'u', authType: 'password', connectedAt: '' };
    const f = fieldsFromHistory(entry);
    expect(f).toMatchObject({ host: 'h', port: '2222', user: 'u', authType: 'password' });
    expect(f.privateKey).toBe('');
    expect(f.jumpHost).toBe('');
  });

  it('defaults authType to vault when missing', () => {
    const entry = { host: 'h', port: 22, user: 'u', connectedAt: '' } as HistoryEntry;
    expect(fieldsFromHistory(entry).authType).toBe('vault');
  });
});

describe('fieldsFromProfile', () => {
  it('carries stored keys and jump config', () => {
    const p: Profile = {
      id: '1', name: 'p', host: 'h', port: 22, user: 'u', authType: 'pubkey', createdAt: '',
      privateKeyContent: 'KEY', privateKeyName: 'id_ed25519',
      jumpHost: 'bastion', jumpPort: 2200, jumpUser: 'j', jumpAuthType: 'vault',
    };
    const f = fieldsFromProfile(p);
    expect(f.privateKey).toBe('KEY');
    expect(f.privateKeyName).toBe('id_ed25519');
    expect(f.jumpHost).toBe('bastion');
    expect(f.jumpPort).toBe('2200');
    expect(f.password).toBe('');
  });

  it('defaults jump port to 22 when unset', () => {
    const p: Profile = { id: '1', name: 'p', host: 'h', port: 22, user: 'u', authType: 'vault', createdAt: '' };
    expect(fieldsFromProfile(p).jumpPort).toBe('22');
  });
});

describe('clearedJumpFields', () => {
  it('resets all jump fields to defaults', () => {
    expect(clearedJumpFields()).toEqual({
      jumpHost: '', jumpPort: '22', jumpUser: '', jumpAuthType: 'vault',
      jumpPassword: '', jumpPrivateKey: '', jumpPrivateKeyName: '', jumpPassphrase: '',
    });
  });
});

describe('buildConnectRequest', () => {
  it('omits jump fields when jumpHost is blank', () => {
    const req = buildConnectRequest({ ...defaultFields(), host: 'h', user: 'u' });
    expect(req).toEqual({ host: 'h', port: 22, user: 'u', auth_type: 'vault' });
  });

  it('includes jump fields when jumpHost is set', () => {
    const req = buildConnectRequest({
      ...defaultFields(), host: 'h', user: 'u',
      jumpHost: 'bastion', jumpPort: '2200', jumpUser: 'j', jumpAuthType: 'vault',
    });
    expect(req.jump_host).toBe('bastion');
    expect(req.jump_port).toBe(2200);
    expect(req.jump_user).toBe('j');
  });
});
