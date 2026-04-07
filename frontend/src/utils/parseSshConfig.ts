export interface SshConfigHost {
  name: string;   // Host エイリアス
  host: string;   // HostName (なければ Host 値)
  port: number;
  user: string;
  // IdentityFile (省略時は undefined)
  identityFile?: string;
  // ProxyJump (省略時は undefined)
  jumpHost?: string;
  jumpPort?: number;
  jumpUser?: string;
  jumpIdentityFile?: string;
}

/** 1 ブロック分の生設定 */
interface RawConfig {
  hostname: string;
  port: number;
  user: string;
  identityFile: string;
}

/**
 * ~/.ssh/config のテキストをパースして接続先の配列を返す。
 * - `Host *` のようなワイルドカードエントリは除外する。
 * - HostName が省略されている場合は Host 値をそのまま使う。
 * - Port が省略されている場合は 22 を使う。
 * - User が省略されている場合は空文字を返す（呼び出し側で補完）。
 * - IdentityFile が設定されていればパスをヒントとして返す。
 * - ProxyJump はカンマ区切りの最初のホストのみを使う。
 *   書式: [user@]host[:port] または別の Host エイリアス名。
 *   エイリアス名の場合は User / Port / HostName を自動解決する。
 */
export function parseSshConfig(text: string): SshConfigHost[] {
  const blocks = text.split(/^Host\s+/im).slice(1);

  // ── 第1パス: 全エイリアスの生設定を収集 ───────────────────────────────────
  const rawByAlias = new Map<string, RawConfig>();

  for (const block of blocks) {
    const lines = block.split('\n');
    const aliasLine = lines[0].trim();
    if (!aliasLine) continue;
    const alias = aliasLine.split(/\s+/)[0];

    let hostname = '';
    let port = 22;
    let user = '';
    let identityFile = '';

    for (const line of lines.slice(1)) {
      const m = line.match(/^\s*(\w+)\s+(.+)$/);
      if (!m) continue;
      const [, key, val] = m;
      switch (key.toLowerCase()) {
        case 'hostname':     hostname     = val.trim(); break;
        case 'port':         port         = parseInt(val.trim(), 10) || 22; break;
        case 'user':         user         = val.trim(); break;
        case 'identityfile': identityFile = val.trim(); break;
      }
    }

    rawByAlias.set(alias, { hostname: hostname || alias, port, user, identityFile });
  }

  // ── 第2パス: ProxyJump を解決して結果を構築 ──────────────────────────────
  const results: SshConfigHost[] = [];

  for (const block of blocks) {
    const lines = block.split('\n');
    const hostAlias = lines[0].trim();

    // ワイルドカードエントリはスキップ
    if (!hostAlias || hostAlias === '*' || hostAlias.includes('*')) continue;

    const name = hostAlias.split(/\s+/)[0];
    const raw  = rawByAlias.get(name);
    if (!raw) continue;

    let jumpHost: string | undefined;
    let jumpPort: number | undefined;
    let jumpUser: string | undefined;
    let jumpIdentityFile: string | undefined;

    for (const line of lines.slice(1)) {
      const m = line.match(/^\s*(\w+)\s+(.+)$/);
      if (!m) continue;
      const [, key, val] = m;

      if (key.toLowerCase() !== 'proxyjump') continue;

      // ProxyJump は最初の1つだけ使う
      if (jumpHost) continue;

      // カンマ区切りの最初のホストのみ使用（多段 jump は未対応）
      const firstHop = val.trim().split(',')[0].trim();
      if (!firstHop || firstHop.toLowerCase() === 'none') continue;

      let jh = firstHop;
      let jp = 22;
      let ju = '';

      // user@ を分離
      if (jh.includes('@')) {
        const at = jh.indexOf('@');
        ju = jh.slice(0, at);
        jh = jh.slice(at + 1);
      }

      // :port を分離（IPv6 以外）
      if (!jh.startsWith('[') && jh.includes(':')) {
        const colon = jh.lastIndexOf(':');
        jp = parseInt(jh.slice(colon + 1), 10) || 22;
        jh = jh.slice(0, colon);
      }

      // ProxyJump 値が Host エイリアスと一致する場合、そこから情報を解決する
      const jumpRaw = rawByAlias.get(jh);
      if (jumpRaw) {
        if (!ju)               ju = jumpRaw.user;         // user@ がなければエイリアスの User を使う
        if (jp === 22)         jp = jumpRaw.port;         // :port がなければエイリアスの Port を使う
        jh = jumpRaw.hostname;                            // HostName に解決
        if (jumpRaw.identityFile) jumpIdentityFile = jumpRaw.identityFile;
      }

      jumpHost = jh;
      jumpPort = jp;
      jumpUser = ju;
    }

    results.push({
      name,
      host: raw.hostname,
      port: raw.port,
      user: raw.user,
      ...(raw.identityFile ? { identityFile: raw.identityFile } : {}),
      ...(jumpHost ? { jumpHost, jumpPort, jumpUser, ...(jumpIdentityFile ? { jumpIdentityFile } : {}) } : {}),
    });
  }

  return results;
}

// ── Diagnostic variant ──────────────────────────────────────────────────────

export interface SshConfigDiagnostic {
  /** Total Host blocks found in the file */
  totalBlocks: number;
  /** Blocks skipped because they are wildcard patterns (Host * / Host *.example) */
  wildcardBlocks: number;
  /** Valid, usable host entries */
  validHosts: SshConfigHost[];
}

/**
 * Parse ~/.ssh/config and return diagnostic information in addition to valid hosts.
 * Useful for composing human-readable error messages when no hosts are importable.
 */
export function parseSshConfigWithDiagnostics(text: string): SshConfigDiagnostic {
  const blocks = text.split(/^Host\s+/im).slice(1);
  const totalBlocks = blocks.length;

  let wildcardBlocks = 0;
  for (const block of blocks) {
    const alias = block.split('\n')[0].trim().split(/\s+/)[0];
    if (!alias) continue;
    if (alias === '*' || alias.includes('*')) wildcardBlocks++;
  }

  const validHosts = parseSshConfig(text);
  return { totalBlocks, wildcardBlocks, validHosts };
}
