// 帮助文档内容：桌面端内嵌展示，不跳浏览器。
//
// 写作要求（别偷懒）：给完全没用过的人看，句子短、动作具体；
// 文里的命令都在 Windows 实机上跑过一遍，输出是真实输出（路径截短），
// Linux 上路径与 reload 方式不同处已单独标注。

const zh = [
  {
    id: 'intro',
    title: '这个程序是干什么的',
    html: `
<p>fast-https 是跑在本机的网站服务：它可以托管静态网页，也可以把请求转发给后端程序（反向代理），还带 HTTPS 证书。</p>
<p>要注意两件事：</p>
<ul>
  <li><b>服务</b>和<b>桌面控制台</b>是两个程序。服务在后台干活，桌面控制台用来管它。关掉控制台，服务照常跑；在控制台里点「停止服务」，服务才会停。</li>
  <li>你在这里做的每次修改（加站点、改配置）都会立刻保存并生效，不用再去改配置文件。</li>
</ul>`
  },
  {
    id: 'first',
    title: '第一次用：五分钟跑起来',
    html: `
<ol>
  <li><b>启动服务。</b>在「概览」页点「启动服务」。状态变成「运行中」，就成功了。</li>
  <li><b>添加站点。</b>到「站点」页，点「+ 添加站点」：
    <ul>
      <li>域名填 <span class="mono">localhost</span>（先在本机试）或你的域名；</li>
      <li>协议和端口：先选「仅 HTTP」，端口 80；</li>
      <li>类型选「静态站点」，根目录选一个放网页的文件夹（比如新建 <span class="mono">D:\\www</span>，里面放个 <span class="mono">index.html</span>）。</li>
    </ul>
    点「创建站点」，完成。
  </li>
  <li><b>打开看看。</b>回到站点列表，点那一行的地址（<span class="mono">http://localhost/</span>），浏览器会打开你的页面。</li>
  <li><b>想用 HTTPS？</b>把协议改成「HTTP + HTTPS」或「仅 HTTPS」。第一次访问会提示「不安全」，这是因为证书是本机自己签的——到「证书」页点「信任本地 CA…」，按弹窗里的三步把证书加进系统信任，再<b>完全退出并重启浏览器</b>就好了。</li>
</ol>`
  },
  {
    id: 'howto',
    title: '常见操作一览',
    html: `
<table class="help-table">
  <tr><th>想做什么</th><th>在哪里做</th></tr>
  <tr><td>加一个新站点</td><td>「站点」页 →「+ 添加站点」</td></tr>
  <tr><td>改端口、改根目录、改转发地址</td><td>「站点」页 → 点站点行末尾的「⋯」，改完点「保存修改」</td></tr>
  <tr><td>临时关掉一个站点（不删除）</td><td>「站点」页 → 点那一行的电源按钮；再点一次恢复</td></tr>
  <tr><td>删除站点</td><td>「站点」页 → 点那一行的垃圾桶按钮</td></tr>
  <tr><td>改全局设置（限流、黑名单、安全头、证书自动签发）</td><td>左下角「本机管理」→「设置」</td></tr>
  <tr><td>看谁在访问、有没有报错</td><td>「日志」页，切 access / error 标签</td></tr>
  <tr><td>看证书什么时候到期</td><td>「证书」页</td></tr>
  <tr><td>检查哪里出了问题</td><td>「概览」页 →「健康检查」，红色项就是问题所在</td></tr>
  <tr><td>看每次改动的记录、必要时回滚</td><td>「概览」页「最近操作」；配置历史在「设置」页「配置版本」，可以对比和回滚</td></tr>
</table>`
  },
  {
    id: 'reload',
    title: '保存之后没生效？',
    html: `
<p>先说结论：改完点过保存，一般都会<b>自动生效</b>。如果刷新浏览器页面还是旧的样子，点一下「重载配置」按钮。</p>
<p>两个平台不太一样：</p>
<ul>
  <li><b>Linux</b>：重载是给服务发个信号，服务不重启、连接不断。</li>
  <li><b>Windows</b>：系统不允许从别的窗口给服务发重载信号。所以「重载配置」按钮在 Windows 上会<b>自动帮你重启服务</b>（停掉再拉起，几秒钟）。正在传输的大文件可能会断，挑不忙的时候点。</li>
</ul>`
  },
  {
    id: 'cli',
    title: '命令行备忘（可选）',
    html: `
<p>平时用界面就够了。想在终端里操作的话，下面几条最常用。命令在安装目录下执行（<span class="mono">fast-https.exe</span> 所在的目录）。</p>
<table class="help-table">
  <tr><th>想做什么</th><th>命令</th></tr>
  <tr><td>看服务在不在跑</td><td class="mono">fast-https status</td></tr>
  <tr><td>启动 / 停止服务</td><td class="mono">fast-https start · fast-https stop</td></tr>
  <tr><td>列出所有站点</td><td class="mono">fast-https site list</td></tr>
  <tr><td>改完配置重新加载</td><td class="mono">fast-https reload</td></tr>
  <tr><td>看看配置、日志都在哪个目录</td><td class="mono">fast-https config path</td></tr>
  <tr><td>看版本</td><td class="mono">fast-https version</td></tr>
</table>
<p>跑起来是这样（输出来自 Windows 实机，路径以你机器为准）：</p>
<pre class="cmd-result">> fast-https status
fast-https is running, pid 19448

> fast-https site list
NAME             LISTEN         SERVER_NAME                  LOCATION
blog.example     8080           blog.example                 / → local …\\httpdoc\\root

> fast-https config path
config   …\\config\\fast-https.json
cert     …\\config\\cert
log      …\\logs
pid      …\\fast-https.pid
root_crt …\\httpdoc\\root\\root.crt
state    …\\state</pre>
<p>两个 Windows 上的坑，提前说：</p>
<ul>
  <li><span class="mono">fast-https start</span> 在 Windows 上是<b>前台运行</b>：窗口会一直占着、实时打日志，按 Ctrl+C 或直接关窗口就是停止服务。Linux 上它会退到后台。</li>
  <li><span class="mono">fast-https reload</span> 在 Windows 上跨窗口发不了信号，会报 <span class="mono">not supported by windows</span>。想重新加载就 <span class="mono">stop</span> 再 <span class="mono">start</span>。Linux 上 reload 正常。</li>
</ul>`
  },
  {
    id: 'faq',
    title: '遇到问题怎么办',
    html: `
<p><b>浏览器一直提示「不安全」。</b>HTTPS 站点第一次访问都这样。把本地 CA 加进系统信任：「证书」页 →「信任本地 CA…」，做完<b>重启浏览器</b>（不是刷新，是把浏览器全关掉再开）。</p>
<p><b>启动时端口被占用。</b>报错里会写明是哪个端口。要么把站点端口改掉（站点行「⋯」），要么找到占用端口的程序停掉它。</p>
<p><b>启动报「can't open mime file」。</b>是 <span class="mono">config</span> 目录里缺 <span class="mono">mime.json</span>。从安装包或发行包里把 <span class="mono">mime.json</span> 拷回 <span class="mono">config</span> 目录就好。</p>
<p><b>服务起不来，不知道为什么。</b>先点「概览」页的「健康检查」，九项检查里标红的那项就是原因；再到「日志」页切 <span class="mono">error</span> 看具体报错。</p>
<p><b>改了配置，页面还是老样子。</b>看上一节「保存之后没生效？」。</p>`
  }
];

const en = [
  {
    id: 'intro',
    title: 'What this program does',
    html: `
<p>fast-https is a web server that runs on your machine. It can host static web pages, forward requests to your own programs (reverse proxy), and serve HTTPS with certificates.</p>
<p>Two things to know:</p>
<ul>
  <li>The <b>service</b> and this <b>desktop console</b> are two separate programs. The service does the work in the background; this console manages it. Closing the console leaves the service running. Click "Stop service" to actually stop it.</li>
  <li>Every change you make here (adding a site, editing settings) is saved and applied right away. You don't edit config files by hand.</li>
</ul>`
  },
  {
    id: 'first',
    title: 'First run: up in five minutes',
    html: `
<ol>
  <li><b>Start the service.</b>On the Overview page click "Start service". When the status turns to "Running", it worked.</li>
  <li><b>Add a site.</b>Go to the Sites page and click "+ Add site":
    <ul>
      <li>Domain: <span class="mono">localhost</span> for a local try, or your real domain;</li>
      <li>Protocol and ports: pick "HTTP only" on port 80 first;</li>
      <li>Type: "Static site", root: pick a folder with your pages (say <span class="mono">D:\\www</span> with an <span class="mono">index.html</span> inside).</li>
    </ul>
    Click "Create site". Done.
  </li>
  <li><b>Open it.</b>Back on the site list, click the address in that row (<span class="mono">http://localhost/</span>) — your browser opens the page.</li>
  <li><b>Want HTTPS?</b>Switch the protocol to "HTTP + HTTPS" or "HTTPS only". The first visit warns "Not secure" because the certificate is signed locally — open the Certificates page, click "Trust local CA…", follow the three steps, then <b>quit the browser completely and reopen it</b>.</li>
</ol>`
  },
  {
    id: 'howto',
    title: 'Everyday tasks',
    html: `
<table class="help-table">
  <tr><th>What you want</th><th>Where to do it</th></tr>
  <tr><td>Add a new site</td><td>Sites page → "+ Add site"</td></tr>
  <tr><td>Change port, root folder, or upstream</td><td>Sites page → the "⋯" button on the site row, then "Save"</td></tr>
  <tr><td>Temporarily turn a site off (without deleting)</td><td>Sites page → the power button on that row; click again to restore</td></tr>
  <tr><td>Delete a site</td><td>Sites page → the trash button on that row</td></tr>
  <tr><td>Change global settings (rate limit, blacklist, security headers, auto certificates)</td><td>"This machine" menu at the bottom left → Settings</td></tr>
  <tr><td>See who is visiting and what failed</td><td>Logs page, switch access / error tabs</td></tr>
  <tr><td>Check when certificates expire</td><td>Certificates page</td></tr>
  <tr><td>Find out what is broken</td><td>Overview page → "Health check"; anything in red is the problem</td></tr>
  <tr><td>Review every change and roll back if needed</td><td>"Recent operations" on Overview; config history lives under Settings → Config versions</td></tr>
</table>`
  },
  {
    id: 'reload',
    title: 'Saved but nothing changed?',
    html: `
<p>Short answer: saving applies the change automatically. If the page still looks old, click "Reload config" once.</p>
<p>The two platforms differ:</p>
<ul>
  <li><b>Linux</b>: reload sends a signal; the service keeps running and connections stay up.</li>
  <li><b>Windows</b>: the system refuses to send reload signals to another window, so "Reload config" <b>restarts the service for you</b> (stop and start, a few seconds). Large downloads in progress may drop — pick a quiet moment.</li>
</ul>`
  },
  {
    id: 'cli',
    title: 'Command line cheat sheet (optional)',
    html: `
<p>The interface covers everything. If you prefer a terminal, these are the commands you will use most. Run them in the install directory (the folder that contains <span class="mono">fast-https.exe</span>).</p>
<table class="help-table">
  <tr><th>What you want</th><th>Command</th></tr>
  <tr><td>Is the service running?</td><td class="mono">fast-https status</td></tr>
  <tr><td>Start / stop the service</td><td class="mono">fast-https start · fast-https stop</td></tr>
  <tr><td>List all sites</td><td class="mono">fast-https site list</td></tr>
  <tr><td>Apply config changes</td><td class="mono">fast-https reload</td></tr>
  <tr><td>Where are the config and logs?</td><td class="mono">fast-https config path</td></tr>
  <tr><td>Show the version</td><td class="mono">fast-https version</td></tr>
</table>
<p>Real output looks like this (from a Windows machine; your paths will differ):</p>
<pre class="cmd-result">> fast-https status
fast-https is running, pid 19448

> fast-https site list
NAME             LISTEN         SERVER_NAME                  LOCATION
blog.example     8080           blog.example                 / → local …\\httpdoc\\root

> fast-https config path
config   …\\config\\fast-https.json
cert     …\\config\\cert
log      …\\logs
pid      …\\fast-https.pid
root_crt …\\httpdoc\\root\\root.crt
state    …\\state</pre>
<p>Two Windows-specific gotchas:</p>
<ul>
  <li><span class="mono">fast-https start</span> runs in the <b>foreground</b> on Windows: the window stays open and prints logs; press Ctrl+C or close the window to stop the service. On Linux it detaches to the background.</li>
  <li><span class="mono">fast-https reload</span> cannot signal a service in another window on Windows and fails with <span class="mono">not supported by windows</span>. Use <span class="mono">stop</span> then <span class="mono">start</span>. Reload works normally on Linux.</li>
</ul>`
  },
  {
    id: 'faq',
    title: 'When something goes wrong',
    html: `
<p><b>The browser keeps saying "Not secure".</b>That is normal for a first HTTPS visit. Trust the local CA: Certificates page → "Trust local CA…", then <b>restart the browser</b> (quit it fully, not just refresh).</p>
<p><b>Startup fails with a port already in use.</b>The error names the port. Either change the site's port (the "⋯" button) or stop whatever holds the port.</p>
<p><b>Startup fails with "can't open mime file".</b>The <span class="mono">config</span> folder is missing <span class="mono">mime.json</span>. Copy <span class="mono">mime.json</span> from the release package back into <span class="mono">config</span>.</p>
<p><b>The service will not start and you do not know why.</b>Run "Health check" on the Overview page first — the red row is the reason. Then read the <span class="mono">error</span> tab on the Logs page.</p>
<p><b>You changed something but the page looks the same.</b>See "Saved but nothing changed?" above.</p>`
  }
];

// helpContent 返回当前语言的帮助内容（分节），语言不认识时回落中文。
// 新增语言：在下面加一份同样结构的数组即可。
export function helpContent(lang) {
  if (lang === 'en') return en;
  return zh;
}
