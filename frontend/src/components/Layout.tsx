import { NavLink, Outlet } from 'react-router-dom';
import { LayoutDashboard, ScrollText, User, Shield, Settings, Sun, Moon, Circle } from 'lucide-react';
import type { Theme } from '../types';

interface Props {
  theme: Theme;
  onToggleTheme: () => void;
}

export function Layout({ theme, onToggleTheme }: Props) {
  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-logo">lingma2api</div>
        <nav className="sidebar-nav">
          <NavLink to="/dashboard" className={({ isActive }) => isActive ? 'active' : ''}>
            <LayoutDashboard size={18} /> 仪表盘
          </NavLink>
          <NavLink to="/logs" className={({ isActive }) => isActive ? 'active' : ''}>
            <ScrollText size={18} /> 请求日志
          </NavLink>
          <NavLink to="/account" className={({ isActive }) => isActive ? 'active' : ''}>
            <User size={18} /> 账号管理
          </NavLink>
          <NavLink to="/policies" className={({ isActive }) => isActive ? 'active' : ''}>
            <Shield size={18} /> 策略引擎
          </NavLink>
          <NavLink to="/settings" className={({ isActive }) => isActive ? 'active' : ''}>
            <Settings size={18} /> 设置
          </NavLink>
        </nav>
        <div className="sidebar-status">
          <Circle size={10} fill="currentColor" style={{ color: 'var(--success)', marginRight: 6, verticalAlign: 'middle' }} />
          已连接
        </div>
      </aside>
      <div className="main-area">
        <div className="top-bar">
          <button className="btn" onClick={onToggleTheme}>
            {theme === 'light' ? <Moon size={16} /> : <Sun size={16} />}
            <span style={{ marginLeft: 6 }}>{theme === 'light' ? '深色' : '浅色'}</span>
          </button>
          <NavLink to="/settings" className="btn">
            <Settings size={16} /> 设置
          </NavLink>
        </div>
        <div className="content">
          <Outlet />
        </div>
        <div className="bottom-bar">lingma2api v1.0.0</div>
      </div>
    </div>
  );
}
