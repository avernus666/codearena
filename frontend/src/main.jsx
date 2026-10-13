import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';
import { tasks as fallbackTasks, filterTasks } from './task-data.js';

const API = (import.meta.env.VITE_API_URL || '').replace(/\/$/, '');
const iconPaths = {
  grid: <><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></>,
  swords: <><path d="m4 4 7 7m-4-9 6 6m-9 2 6 6m10-12-7 7m4-9-6 6m9 2-6 6"/></>,
  chart: <><path d="M4 19V5m0 14h17M7 15l4-4 3 2 6-7"/></>,
  trophy: <><path d="M8 21h8m-4-4v4m-5-18h10v5a5 5 0 0 1-10 0Z"/></>,
  clock: <><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></>,
  search: <><circle cx="10.8" cy="10.8" r="6.8"/><path d="m16 16 5 5"/></>,
  arrow: <path d="M5 12h14m-6-6 6 6-6 6"/>,
  bolt: <path d="m13 2-9 12h7l-1 8 10-13h-7z"/>,
  code: <path d="m8 8-4 4 4 4m8-8 4 4-4 4m-3-11-2 14"/>
};
function Icon({ name }) { return <svg viewBox="0 0 24 24" aria-hidden="true">{iconPaths[name] || iconPaths.grid}</svg>; }
async function api(path, options = {}) {
  const response = await fetch(`${API}/api/v1${path}`, { credentials: 'include', ...options, headers: { 'Content-Type': 'application/json', ...(options.headers || {}) } });
  if (response.status === 204) return null;
  const body = await response.json().catch(() => ({ error: 'Некорректный ответ сервера' }));
  if (!response.ok) throw new Error(body.error || `Ошибка ${response.status}`);
  return body;
}

function TaskCard({ task, onOpen }) {
  const level = task.difficulty || task.level || 'Лёгкая';
  const color = level === 'Сложная' ? 'orange' : level === 'Средняя' ? 'blue' : 'violet';
  return <article className="task-card"><div className={`task-art ${color}`}><span className="task-id">{task.id}</span><span className="art-symbol"><Icon name="code"/></span><span className="art-orbit orbit-one"/><span className="art-orbit orbit-two"/></div><div className="task-content"><div className="task-meta"><span>{task.category || 'Алгоритмы'}</span><span className={`level ${color}`}>{level}</span></div><h3>{task.title}</h3><p className="task-description">{task.description || 'Открой условие и отправь решение.'}</p><div className="task-footer"><span><Icon name="clock"/> {task.timeLimitMs || task.time || 1000} мс</span><button className="icon-button task-open" onClick={() => onOpen(task)} aria-label={`Открыть ${task.title}`}><Icon name="arrow"/></button></div></div></article>;
}

function AuthDialog({ mode, setMode, onClose, onAuth }) {
  const [status, setStatus] = useState('Пароль хранится в виде стойкого хеша на сервере.');
  const [busy, setBusy] = useState(false);
  const register = mode === 'register';
  async function submit(event) {
    event.preventDefault(); setBusy(true); setStatus('');
    const data = new FormData(event.currentTarget);
    const body = register ? { handle: data.get('handle'), email: data.get('email'), password: data.get('password') } : { login: data.get('login'), password: data.get('password') };
    try { const result = await api(register ? '/auth/register' : '/auth/login', { method: 'POST', body: JSON.stringify(body) }); onAuth(result.user); onClose(); }
    catch (error) { setStatus(error.message); }
    finally { setBusy(false); }
  }
  return <div className="modal-backdrop" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}><section className="task-dialog auth-dialog" role="dialog" aria-modal="true" aria-labelledby="auth-title"><button className="dialog-close" onClick={onClose} aria-label="Закрыть">×</button><div className="section-kicker">АККАУНТ ARENA</div><h2 id="auth-title">{register ? <>СОЗДАЙ <span>АККАУНТ</span></> : <>ВХОД <span>В СИСТЕМУ</span></>}</h2><div className="auth-tabs"><button className={`filter-chip ${!register ? 'selected' : ''}`} onClick={() => { setMode('login'); setStatus(''); }}>Вход</button><button className={`filter-chip ${register ? 'selected' : ''}`} onClick={() => { setMode('register'); setStatus(''); }}>Регистрация</button></div><form onSubmit={submit}><label className="form-label" hidden={!register}>Никнейм<input name="handle" autoComplete="username" minLength="3" maxLength="24" required={register}/></label><label className="form-label" hidden={!register}>Email<input name="email" type="email" autoComplete="email" required={register}/></label><label className="form-label" hidden={register}>Никнейм или email<input name="login" autoComplete="username" required={!register}/></label><label className="form-label">Пароль<input name="password" type="password" autoComplete={register ? 'new-password' : 'current-password'} minLength="10" maxLength="128" required/></label><button className="primary-button" type="submit" disabled={busy}>{busy ? 'Подожди…' : register ? 'Создать аккаунт' : 'Войти'} <Icon name="arrow"/></button></form><p className="stat-caption" role="status">{status || (register ? 'Никнейм: 3–24 символа. Пароль: не менее 10 символов.' : 'Войди в аккаунт, чтобы сохранять решения.')}</p></section></div>;
}

function TaskDialog({ task, room, user, onClose, onRefresh, notify }) {
  const [code, setCode] = useState('');
  const [language] = useState('python');
  const [status, setStatus] = useState('Отправление сохранится в истории аккаунта.');
  const [busy, setBusy] = useState(false);
  const [cursor, setCursor] = useState(0);
  const timer = useRef(null);
  const codeRef = useRef(code);
  const roomId = room?.id;
  useEffect(() => { codeRef.current = code; }, [code]);
  useEffect(() => {
    if (!roomId) return undefined;
    let active = true;
    const poll = async () => {
      try {
        const result = await api(`/rooms/${encodeURIComponent(roomId)}/events?after=${cursor}`);
        if (!active) return;
        for (const event of result.items || []) {
          setCursor((previous) => Math.max(previous, Number(event.id) || 0));
          if (event.type === 'editor_update' && event.actorId !== user?.id && typeof event.payload?.code === 'string') {
            const textarea = document.querySelector('#duel-code-editor');
            if (document.activeElement !== textarea) { codeRef.current = event.payload.code; setCode(event.payload.code); }
          }
          if (event.type === 'duel_finished') {
            const latest = await api(`/rooms/${encodeURIComponent(roomId)}`);
            setStatus(latest.winnerId === user?.id ? 'Победа! Рейтинг обновлён.' : 'Дуэль завершена.');
            await onRefresh();
            notify(latest.winnerId === user?.id ? 'Победа! Рейтинг обновлён.' : 'Дуэль завершена.');
          }
        }
      } catch {}
    };
    const interval = window.setInterval(poll, 900);
    poll();
    return () => { active = false; window.clearInterval(interval); window.clearTimeout(timer.current); };
  }, [roomId, cursor, user?.id, onRefresh, notify]);
  function editCode(value) {
    setCode(value); codeRef.current = value;
    if (!roomId) return;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(async () => {
      try { await api(`/rooms/${encodeURIComponent(roomId)}/events`, { method: 'POST', body: JSON.stringify({ type: 'editor_update', payload: { code: codeRef.current } }) }); }
      catch {}
    }, 350);
  }
  async function submit(event) {
    event.preventDefault();
    if (!user) { notify('Для отправки решения нужен аккаунт.'); return; }
    setBusy(true); setStatus('Отправляем решение…');
    try { const result = await api('/submissions', { method: 'POST', body: JSON.stringify({ taskId: task.id, language, code, ...(roomId ? { roomId } : {}) }) }); setStatus(`Отправление принято: ${result.status}.`); notify('Решение отправлено на проверку.'); await onRefresh(); }
    catch (error) { setStatus(error.message); notify(error.message); }
    finally { setBusy(false); }
  }
  return <div className="modal-backdrop" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}><section className="task-dialog task-dialog-wide" role="dialog" aria-modal="true" aria-labelledby="task-dialog-title"><button className="dialog-close" onClick={onClose} aria-label="Закрыть">×</button><div className="section-kicker">{room ? `ДУЭЛЬ · ${room.mode === 'ranked' ? 'RANKED' : 'CASUAL'}` : 'ЗАДАЧА АРЕНЫ'}</div><h2 id="task-dialog-title">{task.title}</h2><p>{task.description}</p><div className="task-spec"><p><b>Ввод:</b> {task.inputFormat || 'См. условие'}</p><p><b>Вывод:</b> {task.outputFormat || 'См. условие'}</p><p><b>Пример ввода</b><pre>{task.sampleInput || ''}</pre></p><p><b>Пример вывода</b><pre>{task.sampleOutput || ''}</pre></p></div><form onSubmit={submit}><label className="form-label">Язык решения<select value={language} disabled><option value="python">Python 3.12</option></select></label><label className="form-label">Код решения<textarea id="duel-code-editor" rows="10" spellCheck="false" value={code} onChange={(e) => editCode(e.target.value)} placeholder={'a, b = map(int, input().split())\nprint(a + b)'}/></label><button className="primary-button" type="submit" disabled={busy}>{busy ? 'Отправляем…' : room ? 'Отправить решение в дуэли' : 'Отправить решение'} <Icon name="arrow"/></button></form><p className="stat-caption" role="status">{status}{room ? ` · Комната ${room.id}` : ''}</p></section></div>;
}

function App() {
  const [tasks, setTasks] = useState(fallbackTasks);
  const [user, setUser] = useState(null);
  const [leaderboard, setLeaderboard] = useState([]);
  const [history, setHistory] = useState([]);
  const [filter, setFilter] = useState('Все');
  const [query, setQuery] = useState('');
  const [view, setView] = useState('home');
  const [authMode, setAuthMode] = useState(null);
  const [activeTask, setActiveTask] = useState(null);
  const [room, setRoom] = useState(null);
  const [toast, setToast] = useState('');
  const [queueing, setQueueing] = useState(false);
  const toastTimer = useRef(null);
  const queueTimer = useRef(null);
  const notify = useCallback((message) => { setToast(message); window.clearTimeout(toastTimer.current); toastTimer.current = window.setTimeout(() => setToast(''), 3200); }, []);
  const refreshUser = useCallback(async () => { try { const result = await api('/auth/me'); setUser(result.user); } catch { setUser(null); } }, []);
  const refreshHistory = useCallback(async () => { if (!user) { setHistory([]); return; } try { const result = await api('/submissions'); setHistory(result.items || []); } catch {} }, [user]);
  const refreshLeaderboard = useCallback(async () => { try { const result = await api('/leaderboard'); setLeaderboard(result.items || []); } catch { setLeaderboard([]); } }, []);
  const refreshAll = useCallback(async () => { await Promise.all([refreshUser(), refreshHistory(), refreshLeaderboard()]); }, [refreshUser, refreshHistory, refreshLeaderboard]);
  useEffect(() => { api('/tasks').then((result) => { if (result.items?.length) setTasks(result.items); }).catch(() => notify('API недоступен: показан локальный каталог задач.')); refreshUser(); refreshLeaderboard(); return () => { window.clearTimeout(toastTimer.current); window.clearInterval(queueTimer.current); }; }, [refreshUser, refreshLeaderboard, notify]);
  useEffect(() => { refreshHistory(); }, [refreshHistory]);
  const visibleTasks = useMemo(() => filterTasks(tasks, query).filter((task) => filter === 'Все' || (task.difficulty || task.level) === filter), [tasks, query, filter]);
  function navigate(next) { setView(next); const selectors = { tasks: '#task-section', rating: '#leaderboard-section', arena: '#activity-section', home: '#home-section' }; document.querySelector(selectors[next] || '#home-section')?.scrollIntoView({ behavior: 'smooth', block: next === 'home' ? 'start' : 'center' }); }
  async function cancelQueue() { try { await api('/matchmaking/queue', { method: 'DELETE' }); } catch {} window.clearInterval(queueTimer.current); setQueueing(false); notify('Поиск соперника отменён.'); }
  async function findOpponent() {
    if (!user) { setAuthMode('login'); notify('Сначала войди или создай аккаунт.'); return; }
    try {
      const result = await api('/matchmaking/queue', { method: 'POST', body: JSON.stringify({ mode: 'ranked' }) });
      if (result.status === 'matched') { await openRoom(result.roomId); return; }
      setQueueing(true); notify('Ты в очереди. Ищем соперника…'); window.clearInterval(queueTimer.current);
      queueTimer.current = window.setInterval(async () => { try { const status = await api('/matchmaking/status'); if (status.status === 'matched') { window.clearInterval(queueTimer.current); setQueueing(false); await openRoom(status.roomId); } else if (status.status === 'idle') { window.clearInterval(queueTimer.current); setQueueing(false); } } catch {} }, 2000);
    } catch (error) { notify(error.message); }
  }
  async function openRoom(roomId) { try { const result = await api(`/rooms/${encodeURIComponent(roomId)}`); let catalog = tasks; let task = catalog.find((item) => item.id === result.taskId); if (!task) { const fetched = await api('/tasks'); catalog = fetched.items || []; setTasks(catalog); task = catalog.find((item) => item.id === result.taskId); } if (!task) throw new Error('Задача дуэли не найдена.'); setRoom(result); setActiveTask(task); notify('Матч найден. Реши задачу раньше соперника.'); } catch (error) { notify(error.message); } }
  async function logout() { try { await api('/auth/logout', { method: 'POST' }); setUser(null); setHistory([]); notify('Ты вышел из аккаунта.'); } catch (error) { notify(error.message); } }
  const displayTasks = visibleTasks.map((task) => <TaskCard key={task.id} task={task} onOpen={setActiveTask}/>);
  return <><div className="app-shell"><aside className="sidebar"><a className="brand" href="#home" onClick={(e) => { e.preventDefault(); navigate('home'); }}><span className="brand-mark">A<span>A</span></span><span className="brand-name">averna<span>arena</span></span></a><div className="sidebar-label">ПЛАТФОРМА</div><nav className="main-nav" aria-label="Основная навигация"><a className={`nav-link ${view === 'home' ? 'active' : ''}`} href="#home" onClick={(e) => { e.preventDefault(); navigate('home'); }}><Icon name="grid"/><span>Главная</span></a><a className={`nav-link ${view === 'arena' ? 'active' : ''}`} href="#arena" onClick={(e) => { e.preventDefault(); navigate('arena'); }}><Icon name="swords"/><span>Арена</span><span className="nav-badge">LIVE</span></a><a className={`nav-link ${view === 'tasks' ? 'active' : ''}`} href="#tasks" onClick={(e) => { e.preventDefault(); navigate('tasks'); }}><Icon name="code"/><span>Задачи</span></a><a className={`nav-link ${view === 'rating' ? 'active' : ''}`} href="#rating" onClick={(e) => { e.preventDefault(); navigate('rating'); }}><Icon name="chart"/><span>Рейтинг</span></a></nav><div className="sidebar-bottom"><div className="season-card"><div className="season-icon">✦</div><div><strong>Сезон 01</strong><span>Путь к вершине</span></div><div className="season-progress"><i/></div><small>Стартовый рейтинг — 1200</small></div><button className="profile-button" onClick={() => user ? logout() : setAuthMode('login')}><span className="avatar">{(user?.handle || '?').slice(0,2).toUpperCase()}</span><span className="profile-copy"><strong>{user?.handle || 'Гость'}</strong><small>{user ? `${user.rating} рейтинга · выйти` : 'Войти или создать аккаунт'}</small></span><span className="dots">···</span></button></div></aside><main className="main-content"><header className="topbar"><div className="breadcrumbs"><span>АРЕНА</span><b>/</b><strong>{({ home:'ОБЗОР', arena:'АРЕНА', tasks:'ЗАДАЧИ', rating:'РЕЙТИНГ' })[view]}</strong></div><div className="top-actions"><span className="online-status"><i/> API-ПЛАТФОРМА</span><button className="user-chip" onClick={() => user ? logout() : setAuthMode('login')}><span className="avatar">{(user?.handle || '?').slice(0,2).toUpperCase()}</span><span>{user?.handle || 'Войти'}</span></button></div></header><section className="hero" id="home-section"><div className="hero-copy"><div className="eyebrow"><span className="eyebrow-line"/> ТВОЯ СЛЕДУЮЩАЯ ПОБЕДА НАЧИНАЕТСЯ ЗДЕСЬ</div><h1>ДОКАЖИ,<br/>ЧТО ТЫ <span>ЛУЧШЕ.</span></h1><p>Соревнуйся в программировании. Решай задачи.<br/>Поднимайся в рейтинге. Оставляй свой след.</p><div className="hero-actions"><button className="primary-button" onClick={queueing ? cancelQueue : findOpponent}>{queueing ? 'Отменить поиск' : 'Найти соперника'} <Icon name="arrow"/></button><button className="secondary-button" onClick={() => navigate('tasks')}>Изучить задачи</button></div><div className="hero-footnote"><span className="live-dot"/> Отправляй Python-решения и соревнуйся в рейтинговых дуэлях</div></div><div className="hero-visual" aria-label="Абстрактная неоновая эмблема арены"><div className="visual-grid"/><div className="visual-ring ring-a"/><div className="visual-ring ring-b"/><div className="visual-core"><div className="core-glyph">A</div></div><div className="visual-label label-top"><span>РЕЖИМ</span><strong>RANKED / 01</strong></div><div className="visual-label label-bottom"><span>ТВОЯ ЦЕЛЬ</span><strong>TOP 1%</strong></div><span className="floating-code">&lt;/&gt;</span><span className="floating-cross">✳</span></div></section><section className="stats-grid"><article className="stat-card"><div className="stat-heading">ТВОЙ РЕЙТИНГ</div><div className="stat-value">{user?.rating ?? '—'}</div><div className="stat-caption">{user ? `${user.wins} побед · ${user.losses} поражений` : 'Войди, чтобы увидеть статистику'}</div></article><article className="stat-card"><div className="stat-heading">ЗАДАЧИ В КАТАЛОГЕ</div><div className="stat-value">{tasks.length}</div><div className="stat-caption">Каталог из PostgreSQL API</div></article><article className="stat-card"><div className="stat-heading">ТВОИ ОТПРАВЛЕНИЯ</div><div className="stat-value">{user ? history.length : '—'}</div><div className="stat-caption">История решений аккаунта</div></article></section><section className="section-block" id="task-section"><div className="section-heading"><div><div className="section-kicker">ВЫБИРАЙ СВОЙ ПУТЬ</div><h2>ЗАДАЧИ <span>НА АРЕНЕ</span></h2></div><button className="text-button" onClick={() => api('/tasks').then((r) => { setTasks(r.items || []); notify('Каталог обновлён.'); }).catch((e) => notify(e.message))}>Обновить <Icon name="arrow"/></button></div><div className="task-toolbar"><div className="task-filters">{['Все','Лёгкая','Средняя','Сложная'].map((value) => <button key={value} className={`filter-chip ${filter === value ? 'selected' : ''}`} onClick={() => setFilter(value)}>{value === 'Все' ? 'Все задачи' : value === 'Лёгкая' ? 'Лёгкие' : value === 'Средняя' ? 'Средние' : 'Сложные'}</button>)}</div><label className="search-box"><Icon name="search"/><input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Найти задачу…" aria-label="Найти задачу"/></label></div><div className="tasks-grid">{displayTasks}</div>{!visibleTasks.length && <div className="empty-state">Ничего не найдено. Попробуй другой запрос.</div>}</section><section className="bottom-grid"><article className="panel leaderboard-panel" id="leaderboard-section"><div className="panel-heading"><div><div className="section-kicker">ГЛОБАЛЬНАЯ ТАБЛИЦА</div><h2>ЛИДЕРЫ <span>АРЕНЫ</span></h2></div><button className="small-link" onClick={refreshLeaderboard}>Обновить ↗</button></div><div className="leader-row leader-head"><span>МЕСТО</span><span>ИГРОК</span><span>РЕЙТИНГ</span><span>W / L</span></div><div>{leaderboard.map((player,index) => <div className="leader-row" key={player.id}><span className={`rank ${index===0?'rank-gold':index===1?'rank-silver':index===2?'rank-bronze':''}`}>{String(index+1).padStart(2,'0')}</span><span className="leader-user"><span className={`leader-avatar ${['gold-avatar','blue-avatar','pink-avatar','green-avatar'][index%4]}`}>{player.handle.slice(0,2).toUpperCase()}</span><span><strong>{player.handle}</strong><small>{index===0?'Лидер арены':'Участник'}</small></span></span><strong>{player.rating}</strong><span>{player.wins} / {player.losses}</span></div>)}{!leaderboard.length && <p className="stat-caption">Создай аккаунт, чтобы попасть в рейтинг.</p>}</div></article><article className="panel activity-panel" id="activity-section"><div className="panel-heading"><div><div className="section-kicker">АККАУНТ</div><h2>МОИ <span>РЕШЕНИЯ</span></h2></div><button className="small-link" onClick={refreshHistory}>Обновить ↗</button></div><div>{history.slice(0,6).map((submission) => <div className="activity-item" key={submission.id}><span className="activity-symbol blue"><Icon name="code"/></span><div><strong>{submission.taskId}</strong><p>{submission.language} · {submission.status}</p><small>{new Date(submission.createdAt).toLocaleString()}</small></div><span className="activity-score">{submission.passedTests || 0}/{submission.totalTests || 0}</span></div>)}{!history.length && <p className="stat-caption">{user ? 'Отправлений пока нет. Выбери задачу.' : 'Войди, чтобы увидеть историю решений.'}</p>}</div><button className="activity-more" onClick={findOpponent}>Найти соперника <Icon name="arrow"/></button></article></section><footer className="footer"><span>© 2026 AVERNAARENA</span><span>BUILT FOR THE COMPETITIVE MIND.</span><span>CONNECTED API <i/></span></footer></main></div>{toast && <div className="toast visible" role="status" aria-live="polite">{toast}</div>}{authMode && <AuthDialog mode={authMode} setMode={setAuthMode} onClose={() => setAuthMode(null)} onAuth={(nextUser) => { setUser(nextUser); notify('Аккаунт готов к работе.'); refreshLeaderboard(); }}/>}{activeTask && <TaskDialog key={`${activeTask.id}-${room?.id || 'practice'}`} task={activeTask} room={room} user={user} onClose={() => { setActiveTask(null); setRoom(null); }} onRefresh={refreshAll} notify={notify}/>}</>;
}

createRoot(document.getElementById('root')).render(<React.StrictMode><App/></React.StrictMode>);
