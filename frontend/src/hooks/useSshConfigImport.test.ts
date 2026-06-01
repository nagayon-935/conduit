import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useSshConfigImport } from './useSshConfigImport';

function fileWith(text: string): File {
  const f = new File([text], 'config');
  // jsdom's File.text() is reliable, but guard for older envs.
  if (typeof f.text !== 'function') {
    (f as unknown as { text: () => Promise<string> }).text = () => Promise.resolve(text);
  }
  return f;
}

const SAMPLE = `Host web\n  HostName 10.0.0.1\n  User ubuntu\n`;

describe('useSshConfigImport', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('reports an error message when no host blocks are found', async () => {
    const importProfiles = vi.fn(() => ({ added: 0, updated: 0 }));
    const { result } = renderHook(() => useSshConfigImport(importProfiles));
    await act(async () => {
      await result.current.onFileChange({
        target: { files: [fileWith('# nothing here')], value: 'x' },
      } as unknown as React.ChangeEvent<HTMLInputElement>);
    });
    expect(importProfiles).not.toHaveBeenCalled();
    expect(result.current.importMessage).toMatch(/No SSH Host entries/);
  });

  it('imports valid hosts and surfaces a summary message', async () => {
    const importProfiles = vi.fn(() => ({ added: 1, updated: 0 }));
    const { result } = renderHook(() => useSshConfigImport(importProfiles));
    await act(async () => {
      await result.current.onFileChange({
        target: { files: [fileWith(SAMPLE)], value: 'x' },
      } as unknown as React.ChangeEvent<HTMLInputElement>);
    });
    expect(importProfiles).toHaveBeenCalledTimes(1);
    expect(result.current.sshConfigFile).not.toBeNull();
    expect(result.current.importMessage).toMatch(/1 added/);
  });

  it('reload re-imports the last file with upsert=true', async () => {
    const importProfiles = vi.fn(() => ({ added: 0, updated: 1 }));
    const { result } = renderHook(() => useSshConfigImport(importProfiles));
    await act(async () => {
      await result.current.onFileChange({
        target: { files: [fileWith(SAMPLE)], value: 'x' },
      } as unknown as React.ChangeEvent<HTMLInputElement>);
    });
    await act(async () => { await result.current.reload(); });
    expect(importProfiles).toHaveBeenLastCalledWith(expect.anything(), true);
  });
});
