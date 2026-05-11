import { useState, useEffect, useCallback } from 'react';
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Layout } from './components/Layout';
import { validateToken } from './api/client';
import { useAdminToken } from './hooks/useAdminToken';
import { useSettings } from './hooks/useSettings';
import { Dashboard } from './pages/Dashboard';
import { Requests } from './pages/Requests';
import { LogDetail } from './pages/LogDetail';
import { Account } from './pages/Account';
import { Policies } from './pages/Policies';
import { Models } from './pages/Models';
import { Settings } from './pages/Settings';

export default function App() {
  const { setToken } = useAdminToken();
  const { theme, setTheme } = useSettings();
  const [authed, setAuthed] = useState(false);
  const [loading, setLoading] = useState(true);

  const checkAuth = useCallback(async () => {
    setLoading(true);
    const ok = await validateToken();
    setAuthed(ok);
    setLoading(false);
  }, []);

  useEffect(() => { checkAuth(); }, [checkAuth]);

  const handleLogin = async (inputToken: string) => {
    setToken(inputToken);
    const ok = await validateToken();
    setAuthed(ok);
    if (!ok) setToken('');
  };

  if (loading) {
    return (
      <div className="auth-shell">
        <div className="auth-card auth-card-loading">加载中...</div>
      </div>
    );
  }

  if (!authed) {
    return (
      <div className="auth-shell">
        <div className="auth-card">
          <div className="auth-kicker">Console Access</div>
          <h2>lingma2api</h2>
          <p>请输入 Admin Token 进入控制台。登录后可管理日志、策略、账户与运行设置。</p>
          <form className="auth-form" onSubmit={(e) => { e.preventDefault(); handleLogin((e.target as HTMLFormElement).token.value); }}>
            <input name="token" className="input" placeholder="Admin Token" />
            <button type="submit" className="btn btn-primary">登录</button>
          </form>
          <div className="auth-footnote">Token 仅保存在当前浏览器的本地存储中。</div>
        </div>
      </div>
    );
  }

  const toggleTheme = () => setTheme(theme === 'light' ? 'dark' : 'light');

  return (
    <HashRouter>
      <Routes>
        <Route element={<Layout theme={theme} onToggleTheme={toggleTheme} />}>
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/requests" element={<Requests />} />
          <Route path="/logs" element={<Navigate to="/requests" replace />} />
          <Route path="/logs/:id" element={<LogDetail />} />
          <Route path="/account" element={<Account />} />
          <Route path="/policies" element={<Policies />} />
          <Route path="/models" element={<Models />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Route>
      </Routes>
    </HashRouter>
  );
}
