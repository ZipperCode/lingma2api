package auth

// CallbackAutoInjectHTML is the HTML returned by GET /auth/callback when the
// 37510 callback server runs in auto-inject mode. The embedded <script> reads
// window.user_info / window.login_url (set by Lingma's callback page in the
// browser context) and POSTs them to /submit-userinfo on the same listener.
const CallbackAutoInjectHTML = `<!DOCTYPE html>
<html><head><meta charset="UTF-8"><title>Lingma Auth</title></head>
<body>
<h1>正在拓印凭据...</h1>
<p id="status">请稍候。本页面将在凭据提交后自动关闭。</p>
<script>
(function(){
  var statusEl = document.getElementById('status');
  try {
    if (typeof window.user_info === 'undefined') {
      statusEl.textContent = '错误：window.user_info 未定义。请检查是否登录完成。';
      return;
    }
    fetch('http://127.0.0.1:37510/submit-userinfo', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        userInfo: typeof window.user_info === 'string'
                  ? window.user_info
                  : JSON.stringify(window.user_info),
        loginUrl: window.login_url || '',
      })
    }).then(function(r){ return r.text(); })
      .then(function(t){ statusEl.textContent = '提交成功，可以关闭窗口。'; })
      .catch(function(e){ statusEl.textContent = '提交失败: ' + e; });
  } catch (e) {
    statusEl.textContent = '脚本错误: ' + e;
  }
})();
</script>
</body></html>`
