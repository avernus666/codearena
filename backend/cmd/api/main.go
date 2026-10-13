package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Server struct {
	db       *sql.DB
	secret   []byte
	judgeURL string
}
type User struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Rating int    `json:"rating"`
	Wins   int    `json:"wins"`
	Losses int    `json:"losses"`
}
type Task struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Category     string `json:"category"`
	Difficulty   string `json:"difficulty"`
	Description  string `json:"description"`
	InputFormat  string `json:"inputFormat"`
	OutputFormat string `json:"outputFormat"`
	SampleInput  string `json:"sampleInput"`
	SampleOutput string `json:"sampleOutput"`
	TimeLimit    int    `json:"timeLimitMs"`
}
type apiError struct {
	Error string `json:"error"`
}

func main() {
	addr := env("ADDR", ":8080")
	db, err := sql.Open("pgx", env("DATABASE_URL", "postgres://averna:averna@localhost:5432/avernaarena?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	s := &Server{db: db, secret: []byte(env("SESSION_SECRET", "change-this-secret-before-deployment")), judgeURL: strings.TrimRight(os.Getenv("JUDGE_URL"), "/")}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/tasks", s.tasks)
	mux.HandleFunc("POST /api/v1/auth/register", s.register)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/me", s.me)
	mux.HandleFunc("GET /api/v1/leaderboard", s.leaderboard)
	mux.HandleFunc("GET /api/v1/submissions", s.submissions)
	mux.HandleFunc("POST /api/v1/submissions", s.submit)
	mux.HandleFunc("POST /api/v1/matchmaking/queue", s.queue)
	mux.HandleFunc("DELETE /api/v1/matchmaking/queue", s.dequeue)
	mux.HandleFunc("GET /api/v1/matchmaking/status", s.queueStatus)
	mux.HandleFunc("GET /api/v1/rooms/{id}", s.room)
	server := &http.Server{Addr: addr, Handler: securityHeaders(cors(mux)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("AvernaArena API listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = db.Close()
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, c := context.WithTimeout(r.Context(), 2*time.Second)
	defer c()
	if err := s.db.PingContext(ctx); err != nil {
		writeError(w, 503, "database unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok", "service": "avernaarena-api"})
}
func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,title,category,difficulty,description,input_format,output_format,sample_input,sample_output,time_limit_ms FROM tasks WHERE published=true ORDER BY sort_order,id`)
	if err != nil {
		writeError(w, 500, "could not load tasks")
		return
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if err = rows.Scan(&t.ID, &t.Title, &t.Category, &t.Difficulty, &t.Description, &t.InputFormat, &t.OutputFormat, &t.SampleInput, &t.SampleOutput, &t.TimeLimit); err != nil {
			writeError(w, 500, "could not read tasks")
			return
		}
		out = append(out, t)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Handle   string `json:"handle"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Handle = strings.TrimSpace(in.Handle)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if len(in.Handle) < 3 || len(in.Handle) > 24 || !validHandle(in.Handle) || len(in.Password) < 10 || len(in.Password) > 128 || !strings.Contains(in.Email, "@") || len(in.Email) > 254 {
		writeError(w, 400, "handle must be 3-24 letters, digits or underscores; valid email and password of 10-128 characters required")
		return
	}
	salt, hash := hashPassword(in.Password)
	var id string
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO users(handle,email,password_salt,password_hash) VALUES($1,$2,$3,$4) RETURNING id`, in.Handle, in.Email, salt, hash).Scan(&id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, 409, "handle or email already exists")
		} else {
			writeError(w, 500, "could not create account")
		}
		return
	}
	s.setSession(w, id, r)
	writeJSON(w, 201, map[string]any{"user": User{ID: id, Handle: in.Handle, Rating: 1200}})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	var u User
	var salt, hash string
	err := s.db.QueryRowContext(r.Context(), `SELECT id,handle,rating,wins,losses,password_salt,password_hash FROM users WHERE lower(handle)=lower($1) OR lower(email)=lower($1)`, strings.TrimSpace(in.Login)).Scan(&u.ID, &u.Handle, &u.Rating, &u.Wins, &u.Losses, &salt, &hash)
	if err != nil || !verifyPassword(in.Password, salt, hash) {
		writeError(w, 401, "invalid credentials")
		return
	}
	s.setSession(w, u.ID, r)
	writeJSON(w, 200, map[string]any{"user": u})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "aa_session", Value: "", Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(204)
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}
func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,handle,rating,wins,losses FROM users ORDER BY rating DESC,wins DESC,created_at ASC LIMIT 100`)
	if err != nil {
		writeError(w, 500, "could not load leaderboard")
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if rows.Scan(&u.ID, &u.Handle, &u.Rating, &u.Wins, &u.Losses) == nil {
			out = append(out, u)
		}
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
func (s *Server) submissions(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,task_id,language,status,created_at FROM submissions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, u.ID)
	if err != nil {
		writeError(w, 500, "could not load submissions")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, task, lang, status string
		var at time.Time
		if rows.Scan(&id, &task, &lang, &status, &at) == nil {
			items = append(items, map[string]any{"id": id, "taskId": task, "language": lang, "status": status, "createdAt": at})
		}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	var in struct {
		TaskID   string `json:"taskId"`
		Language string `json:"language"`
		Code     string `json:"code"`
		RoomID   string `json:"roomId"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Code) == 0 || len(in.Code) > 65536 {
		writeError(w, 400, "code must be between 1 and 65536 bytes")
		return
	}
	if in.Language != "python" {
		writeError(w, 400, "the isolated judge currently supports Python 3.12 only")
		return
	}
	var exists bool
	if err = s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM tasks WHERE id=$1 AND published=true)`, in.TaskID).Scan(&exists); err != nil || !exists {
		writeError(w, 404, "task not found")
		return
	}
	if in.RoomID != "" {
		var roomTask, roomStatus, p1, p2 string
		roomErr := s.db.QueryRowContext(r.Context(), `SELECT task_id,status,player_one,player_two FROM duel_rooms WHERE id=$1`, in.RoomID).Scan(&roomTask, &roomStatus, &p1, &p2)
		if roomErr != nil || (u.ID != p1 && u.ID != p2) {
			writeError(w, 404, "duel room not found")
			return
		}
		if roomStatus != "active" {
			writeError(w, 409, "duel is not active")
			return
		}
		if roomTask != in.TaskID {
			writeError(w, 400, "submission task does not match the duel task")
			return
		}
	}
	var id string
	err = s.db.QueryRowContext(r.Context(), `INSERT INTO submissions(user_id,task_id,language,source_code,status,room_id) VALUES($1,$2,$3,$4,'queued',NULLIF($5,'')::uuid) RETURNING id`, u.ID, in.TaskID, in.Language, in.Code, in.RoomID).Scan(&id)
	if err != nil {
		writeError(w, 500, "could not save submission")
		return
	}
	if s.judgeURL != "" {
		go s.dispatchSubmission(id, in.TaskID, in.Language, in.Code, in.RoomID)
	}
	writeJSON(w, 202, map[string]any{"id": id, "status": "queued", "message": "submission saved; judge status is available in history"})
}
func (s *Server) dispatchSubmission(id, task, lang, code, roomID string) {
	ctx, c := context.WithTimeout(context.Background(), 90*time.Second)
	defer c()
	body, _ := json.Marshal(map[string]string{"submissionId": id, "taskId": task, "language": lang, "code": code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.judgeURL+"/judge", strings.NewReader(string(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_, _ = s.db.Exec(`UPDATE submissions SET status='judge_unavailable' WHERE id=$1 AND status='queued'`, id)
		return
	}
	defer resp.Body.Close()
	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Passed  int    `json:"passed"`
		Total   int    `json:"total"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return
	}
	if result.Status != "accepted" && result.Status != "wrong_answer" && result.Status != "compile_error" && result.Status != "runtime_error" && result.Status != "time_limit" {
		result.Status = "judge_unavailable"
	}
	_, _ = s.db.Exec(`UPDATE submissions SET status=$2,judge_message=$3,passed_tests=$4,total_tests=$5 WHERE id=$1`, id, result.Status, result.Message, result.Passed, result.Total)
	if result.Status == "accepted" && roomID != "" {
		s.finishDuel(roomID, id)
	}
}
func (s *Server) queue(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	var in struct {
		Mode string `json:"mode"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Mode != "ranked" && in.Mode != "casual" {
		writeError(w, 400, "mode must be ranked or casual")
		return
	}
	var existingRoom string
	if err = s.db.QueryRowContext(r.Context(), `SELECT id FROM duel_rooms WHERE status='active' AND (player_one=$1 OR player_two=$1) ORDER BY created_at DESC LIMIT 1`, u.ID).Scan(&existingRoom); err == nil {
		writeJSON(w, 200, map[string]string{"status": "matched", "roomId": existingRoom})
		return
	} else if err != sql.ErrNoRows {
		writeError(w, 500, "could not check active duel")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "could not join queue")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM matchmaking_queue WHERE user_id=$1`, u.ID); err != nil {
		writeError(w, 500, "could not update queue")
		return
	}
	var opponent string
	err = tx.QueryRowContext(r.Context(), `SELECT user_id FROM matchmaking_queue WHERE mode=$1 AND user_id<>$2 ORDER BY joined_at LIMIT 1 FOR UPDATE SKIP LOCKED`, in.Mode, u.ID).Scan(&opponent)
	if err == nil {
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM matchmaking_queue WHERE user_id=$1 OR user_id=$2`, u.ID, opponent)
		var roomID, taskID string
		if err = tx.QueryRowContext(r.Context(), `SELECT id FROM tasks WHERE published=true ORDER BY random() LIMIT 1`).Scan(&taskID); err != nil {
			writeError(w, 500, "no tasks available for duel")
			return
		}
		err = tx.QueryRowContext(r.Context(), `INSERT INTO duel_rooms(mode,status,player_one,player_two,task_id) VALUES($1,'active',$2,$3,$4) RETURNING id`, in.Mode, opponent, u.ID, taskID).Scan(&roomID)
		if err != nil {
			writeError(w, 500, "could not create duel room")
			return
		}
		if err = tx.Commit(); err != nil {
			writeError(w, 500, "could not finalize matchmaking")
			return
		}
		writeJSON(w, 201, map[string]any{"status": "matched", "roomId": roomID, "opponentId": opponent})
		return
	}
	if err != sql.ErrNoRows {
		writeError(w, 500, "could not search for opponent")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO matchmaking_queue(user_id,mode) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET mode=EXCLUDED.mode,joined_at=now()`, u.ID, in.Mode)
	if err != nil {
		writeError(w, 500, "could not join queue")
		return
	}
	if err = tx.Commit(); err != nil {
		writeError(w, 500, "could not finalize queue")
		return
	}
	writeJSON(w, 202, map[string]string{"status": "queued", "mode": in.Mode})
}
func (s *Server) dequeue(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `DELETE FROM matchmaking_queue WHERE user_id=$1`, u.ID)
	if err != nil {
		writeError(w, 500, "could not leave queue")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) queueStatus(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	var roomID string
	roomErr := s.db.QueryRowContext(r.Context(), `SELECT id FROM duel_rooms WHERE (player_one=$1 OR player_two=$1) AND status='active' ORDER BY created_at DESC LIMIT 1`, u.ID).Scan(&roomID)
	if roomErr == nil {
		writeJSON(w, 200, map[string]string{"status": "matched", "roomId": roomID})
		return
	}
	var mode string
	err = s.db.QueryRowContext(r.Context(), `SELECT mode FROM matchmaking_queue WHERE user_id=$1`, u.ID).Scan(&mode)
	if err == sql.ErrNoRows {
		writeJSON(w, 200, map[string]string{"status": "idle"})
		return
	}
	if err != nil {
		writeError(w, 500, "could not check queue")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "queued", "mode": mode})
}
func (s *Server) room(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	var id, mode, status, p1, p2, taskID string
	var winner sql.NullString
	var created time.Time
	err = s.db.QueryRowContext(r.Context(), `SELECT id,mode,status,player_one,player_two,task_id,winner_id,created_at FROM duel_rooms WHERE id=$1`, r.PathValue("id")).Scan(&id, &mode, &status, &p1, &p2, &taskID, &winner, &created)
	if err != nil {
		writeError(w, 404, "room not found")
		return
	}
	if u.ID != p1 && u.ID != p2 {
		writeError(w, 403, "room access denied")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "mode": mode, "status": status, "taskId": taskID, "winnerId": winner.String, "createdAt": created, "players": []string{p1, p2}})
}

func (s *Server) roomEvents(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	var p1, p2 string
	if err = s.db.QueryRowContext(r.Context(), `SELECT player_one,player_two FROM duel_rooms WHERE id=$1`, r.PathValue("id")).Scan(&p1, &p2); err != nil {
		writeError(w, 404, "room not found")
		return
	}
	if u.ID != p1 && u.ID != p2 {
		writeError(w, 403, "room access denied")
		return
	}
	after := r.URL.Query().Get("after")
	if after == "" {
		after = "0"
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,actor_id,event_type,payload,created_at FROM duel_events WHERE room_id=$1 AND id>$2 ORDER BY id LIMIT 100`, r.PathValue("id"), after)
	if err != nil {
		writeError(w, 500, "could not load room events")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var actor, kind string
		var payload []byte
		var created time.Time
		if rows.Scan(&id, &actor, &kind, &payload, &created) == nil {
			var data any
			_ = json.Unmarshal(payload, &data)
			items = append(items, map[string]any{"id": id, "actorId": actor, "type": kind, "payload": data, "createdAt": created})
		}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) addRoomEvent(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		writeError(w, 401, "authentication required")
		return
	}
	var in struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Payload) == 0 || len(in.Payload) > 16*1024 || (in.Type != "editor_update" && in.Type != "message") {
		writeError(w, 400, "invalid event type or payload size")
		return
	}
	var p1, p2, status string
	if err = s.db.QueryRowContext(r.Context(), `SELECT player_one,player_two,status FROM duel_rooms WHERE id=$1`, r.PathValue("id")).Scan(&p1, &p2, &status); err != nil {
		writeError(w, 404, "room not found")
		return
	}
	if u.ID != p1 && u.ID != p2 {
		writeError(w, 403, "room access denied")
		return
	}
	if status != "active" {
		writeError(w, 409, "duel is not active")
		return
	}
	var id int64
	err = s.db.QueryRowContext(r.Context(), `INSERT INTO duel_events(room_id,actor_id,event_type,payload) VALUES($1,$2,$3,$4::jsonb) RETURNING id`, r.PathValue("id"), u.ID, in.Type, string(in.Payload)).Scan(&id)
	if err != nil {
		writeError(w, 500, "could not save room event")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}
func (s *Server) finishDuel(roomID, submissionID string) {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	var winner, loser, mode string
	err = tx.QueryRow(`UPDATE duel_rooms SET status='finished',winner_id=(SELECT user_id FROM submissions WHERE id=$2),finished_at=now() WHERE id=$1 AND status='active' AND EXISTS(SELECT 1 FROM submissions WHERE id=$2 AND room_id=$1 AND status='accepted') RETURNING winner_id,CASE WHEN player_one=winner_id THEN player_two ELSE player_one END,mode`, roomID, submissionID).Scan(&winner, &loser, &mode)
	if err != nil {
		return
	}
	var ratingWinner, ratingLoser int
	if err = tx.QueryRow(`SELECT rating FROM users WHERE id=$1`, winner).Scan(&ratingWinner); err != nil {
		return
	}
	if err = tx.QueryRow(`SELECT rating FROM users WHERE id=$1`, loser).Scan(&ratingLoser); err != nil {
		return
	}
	delta := 0
	if mode == "ranked" {
		delta = eloDelta(ratingWinner, ratingLoser)
	}
	if _, err = tx.Exec(`UPDATE users SET rating=rating+$2,wins=wins+1 WHERE id=$1`, winner, delta); err != nil {
		return
	}
	if _, err = tx.Exec(`UPDATE users SET rating=GREATEST(0,rating-$2),losses=losses+1 WHERE id=$1`, loser, delta); err != nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{"winnerId": winner, "loserId": loser, "ratingChange": delta, "mode": mode})
	if _, err = tx.Exec(`INSERT INTO duel_events(room_id,actor_id,event_type,payload) VALUES($1,$2,'duel_finished',$3::jsonb)`, roomID, winner, string(payload)); err != nil {
		return
	}
	_ = tx.Commit()
}

func eloDelta(winnerRating, loserRating int) int {
	expected := 1.0 / (1.0 + math.Pow(10, float64(loserRating-winnerRating)/400.0))
	delta := int(math.Round(32 * (1 - expected)))
	if delta < 1 {
		return 1
	}
	return delta
}

func (s *Server) currentUser(r *http.Request) (User, error) {
	c, err := r.Cookie("aa_session")
	if err != nil {
		return User{}, err
	}
	id, err := s.verifySession(c.Value)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.db.QueryRowContext(r.Context(), `SELECT id,handle,rating,wins,losses FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Handle, &u.Rating, &u.Wins, &u.Losses)
	return u, err
}
func (s *Server) setSession(w http.ResponseWriter, id string, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "aa_session", Value: s.signSession(id), Path: "/", HttpOnly: true, Secure: isSecure(r), SameSite: http.SameSiteLaxMode, MaxAge: 60 * 60 * 24 * 14})
}
func isSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true")
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid request body")
		return false
	}
	return true
}
func validHandle(s string) bool {
	if len(s) < 3 || len(s) > 24 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := os.Getenv("CORS_ORIGIN")
		if origin != "" && r.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}
