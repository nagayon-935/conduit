import { useState, useRef, useEffect, useCallback } from 'react';
import type { ImportProfilesFn } from './useProfiles';
import { parseSshConfigWithDiagnostics } from '../utils/parseSshConfig';

export interface UseSshConfigImportResult {
  sshConfigFile: File | null;
  importMessage: string | null;
  fileInputRef: React.RefObject<HTMLInputElement>;
  openFilePicker: () => void;
  reload: () => Promise<void>;
  onFileChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
}

/**
 * Encapsulates importing hosts from an ~/.ssh/config file: parsing, diagnostic
 * messaging, and remembering the last file for reload.
 */
export function useSshConfigImport(importProfiles: ImportProfilesFn): UseSshConfigImportResult {
  const [sshConfigFile, setSshConfigFile] = useState<File | null>(null);
  const [importMessage, setImportMessage] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const msgTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (msgTimerRef.current) clearTimeout(msgTimerRef.current);
  }, []);

  const flashMessage = useCallback((msg: string, durationMs: number) => {
    setImportMessage(msg);
    if (msgTimerRef.current) clearTimeout(msgTimerRef.current);
    msgTimerRef.current = setTimeout(() => setImportMessage(null), durationMs);
  }, []);

  const processConfigFile = useCallback(async (file: File, upsert: boolean) => {
    const text = await file.text();
    const { totalBlocks, wildcardBlocks, validHosts } = parseSshConfigWithDiagnostics(text);

    if (totalBlocks === 0) {
      flashMessage('No SSH Host entries found. Is this an SSH config file?', 5000);
      return;
    }
    if (validHosts.length === 0) {
      const msg = wildcardBlocks === totalBlocks
        ? `Found ${totalBlocks} entr${totalBlocks === 1 ? 'y' : 'ies'}, but all are wildcard patterns (Host *) — no specific hosts to import.`
        : `No importable hosts found in ${totalBlocks} entr${totalBlocks === 1 ? 'y' : 'ies'}. Each entry needs at least Host and User fields.`;
      flashMessage(msg, 6000);
      return;
    }

    const { added, updated } = importProfiles(validHosts, upsert);
    const parts: string[] = [];
    if (added > 0) parts.push(`${added} added`);
    if (updated > 0) parts.push(`${updated} updated`);
    const skipped = totalBlocks - wildcardBlocks - validHosts.length;
    const suffix = wildcardBlocks > 0 || skipped > 0
      ? ` (${[wildcardBlocks > 0 && `${wildcardBlocks} wildcard`, skipped > 0 && `${skipped} incomplete`].filter(Boolean).join(', ')} skipped)`
      : '';
    flashMessage(
      parts.length > 0 ? `Imported: ${parts.join(', ')}.${suffix}` : `No changes.${suffix}`,
      4000,
    );
  }, [importProfiles, flashMessage]);

  const openFilePicker = useCallback(() => fileInputRef.current?.click(), []);

  const reload = useCallback(async () => {
    if (sshConfigFile) await processConfigFile(sshConfigFile, true);
  }, [sshConfigFile, processConfigFile]);

  const onFileChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setSshConfigFile(file);
    processConfigFile(file, false);
    e.target.value = '';
  }, [processConfigFile]);

  return { sshConfigFile, importMessage, fileInputRef, openFilePicker, reload, onFileChange };
}
