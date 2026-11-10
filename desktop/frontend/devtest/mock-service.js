// 点击测试台的 DesktopService 假实现：返回贴近 model.go 契约的样例数据，
// 让页面在纯浏览器里能渲染、能点。字段名与 desktop/internal/model/*.go
// 的 json tag 一致，改契约时这里要跟着改。
export const DesktopService = {
  async GetOverview() {
    return {
      status: 'running', running: true, pid: 1234, version: 'fast-https V1.7.0',
      uptimeSeconds: 5400, configVersion: 'v013',
      ports: [':80', ':443'], siteCount: 2, tlsCount: 1, nearestCertDays: 80,
      configDirty: false,
      paths: {
        config: 'C:\\demo\\config\\fast-https.json', cert: 'C:\\demo\\config\\cert',
        log: 'C:\\demo\\logs', pid: 'C:\\demo\\fast-https.pid',
        root_crt: 'C:\\demo\\httpdoc\\root\\root.crt', state: 'C:\\demo\\state'
      },
      checkedAt: '12:00:00', error: ''
    };
  },
  async ListSites() {
    return [
      { id: 'blog', name: 'blog.example.com', port: ':80', listen: '80', tls: false, source: 'C:\\demo\\config\\conf.d\\blog.json', enabled: true, certStatus: '', error: '' },
      { id: 'blog', name: 'blog.example.com', port: ':443', listen: '443 ssl h2', tls: true, source: 'C:\\demo\\config\\conf.d\\blog.json', enabled: true, certStatus: 'valid', error: '' },
      { id: 'api', name: 'api.example.com', port: ':8080', listen: '8080', tls: false, source: 'C:\\demo\\config\\conf.d\\api.json.disabled', enabled: false, certStatus: 'expiring', error: '' }
    ];
  },
  async ListCerts() {
    return [{
      domain: 'blog.example.com', subject: 'CN=blog.example.com', names: 'blog.example.com, www.example.com',
      issuer: 'CN=fast-https local CA', notAfter: '2027-04-01', daysLeft: 80,
      selfSigned: false, local: true, status: 'valid', acmeManaged: false,
      file: 'C:\\demo\\config\\cert\\blog.example.com.pem', note: 'C:\\demo\\config\\cert\\blog.example.com.pem'
    }];
  },
  async TailLog() {
    return [
      { time: '2026-11-10T02:20:05.123', level: 'info', kind: 'access', message: 'request served', method: 'GET', path: '/index.html', status: '200', size: '1234', host: 'blog.example.com', remoteAddr: '127.0.0.1', raw: '{}' },
      { time: '2026-11-10T02:19:05.123', level: 'info', kind: 'access', message: 'request served', method: 'POST', path: '/api/ping', status: '500', size: '12', host: 'api.example.com', remoteAddr: '127.0.0.1', raw: '{}' }
    ];
  },
  async QueryLogs() {
    return {
      lines: [
        { time: '2026-11-10T02:20:05.123', level: 'info', kind: 'access', message: 'request served', method: 'GET', path: '/index.html', status: '200', size: '1234', host: 'blog.example.com', remoteAddr: '127.0.0.1', raw: '{"time":"2026-11-10T02:20:05.123"}' },
        { time: '2026-11-10T02:18:05.123', level: 'error', kind: 'error', message: 'upstream timeout', method: '', path: '', status: '', size: '', host: 'api.example.com', remoteAddr: '', raw: '{}' }
      ],
      hasMore: true, nextUntil: '2026-11-10T02:18:05.123'
    };
  },
  async ReadSiteDetail() {
    return {
      id: 'blog', source: 'C:\\demo\\config\\conf.d\\blog.json', path: 'conf.d/blog.json',
      editable: true, note: '',
      name: 'blog.example.com www.example.com', listen: '80, 443 ssl h2', type: 'proxy',
      root: '', proxyPass: 'http://127.0.0.1:8080',
      cert: './cert/blog.example.com.pem', certKey: './cert/blog.example.com-key.pem', index: 'index.html index.htm',
      raw: '{"server_name":"blog.example.com www.example.com"}'
    };
  },
  async AddSite() { return '已创建站点 blog.example.com（操作 op_001，配置版本 v014）。配置已通过事务提交。'; },
  async SaveSiteDetail() { return '已更新站点 blog（操作 op_002，配置版本 v015）。配置已通过事务提交。'; },
  async DeleteSite() { return '已删除站点 api（操作 op_003，配置版本 v016）。配置已通过事务提交。'; },
  async SetSiteEnabled() { return '已停用站点 api（操作 op_004，配置版本 v017）。配置已通过事务提交。'; },
  async ReadConfigForm() {
    return {
      path: 'C:\\demo\\config\\fast-https.json', size: 2048, modified: '2026-11-10 02:00:00', writable: true,
      preserved: ['engine'],
      fields: [
        { path: 'security.host_check', label: 'Host 校验', group: '安全', type: 'select', value: 'off', options: ['off', 'warn', 'enforce'], help: '' },
        { path: 'acme.enabled', label: '启用自动签发', group: 'ACME', type: 'bool', value: 'false', missing: true },
        { path: 'acme.email', label: 'ACME 邮箱', group: 'ACME', type: 'email', value: '', missing: true },
        { path: 'http.default_type', label: '默认 MIME 类型', group: 'HTTP', type: 'text', value: 'application/octet-stream' },
        { path: 'http.include', label: 'include 目录 / 文件', group: 'HTTP', type: 'list', value: './conf.d' }
      ]
    };
  },
  async SaveConfigForm() { return '已保存 2 个字段并应用（配置事务完成）（操作 op_005，配置版本 v018，恢复点 v017）。配置已通过事务提交。'; },
  async Paths() {
    return {
      config: 'C:\\demo\\config\\fast-https.json', cert: 'C:\\demo\\config\\cert',
      log: 'C:\\demo\\logs', pid: 'C:\\demo\\fast-https.pid',
      root_crt: 'C:\\demo\\httpdoc\\root\\root.crt', state: 'C:\\demo\\state'
    };
  },
  async StartServer() { return '启动指令已发送，状态会在 2 秒后刷新'; },
  async StopServer() { return 'fast-https stopped'; },
  async ReloadServer() { return '重载完成并已验证（操作 op_006）'; },
  async ServerStatus() { return 'fast-https is running, pid 1234'; },
  async OpenPath(p) { return '已打开 ' + p; },
  async OpenHelp() { return '已打开 docs/usage.html'; },
  async ImportLocalCA() { return '已向系统发起导入请求'; },
  async CertTrustGuide() {
    return {
      platform: 'windows', platformLabel: 'Windows',
      certPath: 'C:\\demo\\httpdoc\\root\\root.crt', dirPath: 'C:\\demo\\httpdoc\\root', keyPath: 'C:\\demo\\config\\cert\\root.key',
      exists: true, command: 'certutil -addstore -f ROOT "C:\\demo\\httpdoc\\root\\root.crt"',
      steps: ['用管理员权限执行上方命令', '确认 certutil 报告已加入 ROOT 存储', '重启浏览器'],
      risk: '把这个根 CA 加入系统信任库后，本机所有 HTTPS 站点都会信任由它签发的证书。'
    };
  },
  async PickPath() { return ''; },
  async MinimiseWindow() { }, async ToggleMaximiseWindow() { }, async HideWindow() { }, async QuitApp() { },
  async RunCommand(args) {
    const key = args.slice(0, 2).join(' ');
    const data = {
      'operation list': { operations: [
        { operation_id: 'op_001', command: 'site.add', status: 'succeeded', created_at: '2026-11-10T02:20:00Z', phase: 'committed', result: { version: 'v014', restore_point: 'v013', config_state: 'matches_candidate' }, error: null },
        { operation_id: 'op_002', command: 'config.apply', status: 'succeeded', created_at: '2026-11-10T02:18:00Z', phase: 'committed', result: { version: 'v013' }, error: null }
      ] },
      'operation get': { operation_id: 'op_001', command: 'site.add', status: 'succeeded', created_at: '2026-11-10T02:20:00Z', phase: 'committed', result: { version: 'v014' }, error: null },
      'config versions': { versions: [
        { version: 'v013', created_at: '2026-11-10T02:18:00Z', created_by: 'desktop', summary: 'config apply', active: true },
        { version: 'v012', created_at: '2026-11-10T02:10:00Z', created_by: 'desktop', summary: 'site add', active: false }
      ], drift: false },
      'config diff': { identical: false, changes: [
        { path: 'http.servers_limit.limit', op: 'modified', old: 0, new: 100, changed: true },
        { path: 'acme.email', op: 'modified', changed: true }
      ] },
      'config rollback': { dry_run: false, operation_id: 'op_009', status: 'succeeded', version: 'v012', restore_point: 'v013' },
      'config validate': { valid: true, file: 'C:\\demo\\config\\fast-https.json', candidate: false },
      'runtime status': { observed_at: '2026-11-10T02:20:00Z', source: 'runtime', state: 'running', pid: 1234, uptime_seconds: 5400, connections: { active: 3, accepted_total: 1000, closed_total: 997, rejected_total: 0 }, requests: { active: 1, total: 5000 } },
      'runtime stats': { observed_at: '2026-11-10T02:20:00Z', source: 'runtime', window: 'since_process_start', requests: { active: 1, total: 5000, parse_errors_total: 0, aborted_total: 0 }, responses: { '2xx_total': 4800, '3xx_total': 100, '4xx_total': 90, '5xx_total': 10, unknown_total: 0 }, upstream: { dial_fail_total: 0, idle_conns: 2 } },
      'monitor status': { status: 'running', observed_at: '2026-11-10T02:20:00Z', source: 'monitor_status_file', version: '1.0.0', pid: 2345, healthy: true },
      'monitor events': { events: [
        { time: '2026-11-10T02:15:00Z', type: 'cert_renew', severity: 'warning', message: 'certificate expiring soon' }
      ], skipped: 0, truncated: false },
      'diagnose': { observed_at: '2026-11-10T02:20:00Z', overall: 'warning', report_id: 'rep_001', checks: [
        { name: 'service', status: 'ok', message: 'running, pid 1234' },
        { name: 'config', status: 'ok', message: 'config is valid' },
        { name: 'config_drift', status: 'unavailable', message: 'no pid file to compare against' },
        { name: 'certs', status: 'warning', message: '1 certificate file(s) expiring' },
        { name: 'ports', status: 'ok', message: 'all declared ports reachable' },
        { name: 'logs', status: 'ok', message: 'log files writable' },
        { name: 'runtime', status: 'ok', message: 'runtime snapshot reachable' },
        { name: 'monitor', status: 'ok', message: 'monitor status file present' },
        { name: 'state', status: 'ok', message: 'state dir writable' }
      ] },
      'cert check': {
        domain: 'blog.example.com', passed: true,
        certificate: { domains: ['blog.example.com'], file: 'C:\\demo\\config\\cert\\blog.example.com.pem', status: 'valid', days_left: 80 },
        checks: { exists: { checked: true, ok: true }, validity: { checked: true, ok: true, detail: 'valid' }, hostname: { checked: true, ok: true }, key_pair: { checked: true, ok: true }, chain: { checked: false, certificates_in_file: 1 } }
      }
    }[key] ?? {};
    if (key === 'acme show') {
      return { command: 'acme show', text: '状态：关闭\n邮箱：(空)\n挑战：auto\n续期提前：30 天', warnings: [] };
    }
    return { command: args.join(' '), data, warnings: [] };
  }
};
