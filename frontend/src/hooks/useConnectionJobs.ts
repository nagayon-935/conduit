import { useCallback, useEffect, useRef, useState } from 'react';
import { connectToHost } from '../api/connect';
import type { ConnectResponse } from '../types';
import { buildConnectRequest, type FormFields } from '../utils/form';

export interface ConnectionJob { id: string; host: string; user: string; port: string; state: 'connecting' | 'connected' | 'failed' | 'cancelled'; error?: string }

export function useConnectionJobs(onConnected: (response: ConnectResponse, fields: FormFields) => void) {
  const [jobs, setJobs] = useState<ConnectionJob[]>([]);
  const callback = useRef(onConnected);
  callback.current = onConnected;
  const requests = useRef(new Map<string, { fields: FormFields; controller: AbortController }>());
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      requests.current.forEach((req) => req.controller.abort());
      requests.current.clear();
    };
  }, []);
  const run = useCallback(async (id: string, fields: FormFields) => {
    const controller = new AbortController();
    requests.current.set(id, { fields: { ...fields }, controller });
    setJobs((prev) => prev.map((job) => job.id === id ? { ...job, state: 'connecting', error: undefined } : job));
    try {
      const response = await connectToHost(buildConnectRequest(fields), controller.signal);
      if (!mounted.current || controller.signal.aborted) return;
      callback.current(response, fields);
      setJobs((prev) => prev.map((job) => job.id === id ? { ...job, state: 'connected' } : job));
      requests.current.delete(id); // release credentials as soon as they are no longer needed
    } catch (error) {
      if (!mounted.current) return;
      const cancelled = controller.signal.aborted;
      setJobs((prev) => prev.map((job) => job.id === id ? { ...job, state: cancelled ? 'cancelled' : 'failed',
        error: error instanceof Error ? error.message : '接続に失敗しました。' } : job));
      if (cancelled) requests.current.delete(id);
    }
  }, []);
  const connectEntries = useCallback((entries: FormFields[]) => {
    const newJobs: ConnectionJob[] = entries.map((fields) => ({ id: crypto.randomUUID(), host: fields.host, user: fields.user, port: fields.port, state: 'connecting' }));
    setJobs((prev) => [...prev, ...newJobs]);
    newJobs.forEach((job, index) => { void run(job.id, entries[index]); });
  }, [run]);
  const retry = useCallback((id: string) => {
    const request = requests.current.get(id);
    if (request && !request.controller.signal.aborted) void run(id, request.fields);
  }, [run]);
  const cancel = useCallback((id: string) => { requests.current.get(id)?.controller.abort(); }, []);
  const dismiss = useCallback((id: string) => {
    requests.current.get(id)?.controller.abort(); requests.current.delete(id);
    setJobs((prev) => prev.filter((job) => job.id !== id));
  }, []);
  const edit = useCallback((id: string) => {
    const fields = requests.current.get(id)?.fields;
    return fields ? { ...fields } : undefined;
  }, []);
  return { jobs, connectEntries, retry, cancel, dismiss, edit };
}
