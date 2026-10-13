CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS users (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 handle VARCHAR(24) NOT NULL UNIQUE,
 email VARCHAR(254) NOT NULL UNIQUE,
 password_salt VARCHAR(64) NOT NULL,
 password_hash VARCHAR(128) NOT NULL,
 rating INTEGER NOT NULL DEFAULT 1200 CHECK (rating >= 0),
 wins INTEGER NOT NULL DEFAULT 0 CHECK (wins >= 0),
 losses INTEGER NOT NULL DEFAULT 0 CHECK (losses >= 0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS tasks (
 id VARCHAR(48) PRIMARY KEY,
 title VARCHAR(120) NOT NULL,
 category VARCHAR(60) NOT NULL,
 difficulty VARCHAR(16) NOT NULL CHECK (difficulty IN ('Лёгкая','Средняя','Сложная')),
 description TEXT NOT NULL,
 input_format TEXT NOT NULL,
 output_format TEXT NOT NULL,
 sample_input TEXT NOT NULL,
 sample_output TEXT NOT NULL,
 time_limit_ms INTEGER NOT NULL DEFAULT 1000 CHECK (time_limit_ms BETWEEN 100 AND 10000),
 published BOOLEAN NOT NULL DEFAULT true,
 sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS submissions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 task_id VARCHAR(48) NOT NULL REFERENCES tasks(id),
 language VARCHAR(16) NOT NULL CHECK (language IN ('python','go','rust')),
 source_code TEXT NOT NULL CHECK (octet_length(source_code) <= 65536),
 status VARCHAR(32) NOT NULL DEFAULT 'queued',
 judge_message TEXT NOT NULL DEFAULT '',
 passed_tests INTEGER NOT NULL DEFAULT 0,
 total_tests INTEGER NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS submissions_user_created_idx ON submissions(user_id,created_at DESC);
CREATE TABLE IF NOT EXISTS matchmaking_queue (
 user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 mode VARCHAR(16) NOT NULL CHECK (mode IN ('ranked','casual')),
 joined_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS matchmaking_queue_mode_time_idx ON matchmaking_queue(mode,joined_at);
CREATE TABLE IF NOT EXISTS duel_rooms (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 mode VARCHAR(16) NOT NULL CHECK (mode IN ('ranked','casual')),
 status VARCHAR(24) NOT NULL DEFAULT 'waiting',
 player_one UUID NOT NULL REFERENCES users(id),
 player_two UUID NOT NULL REFERENCES users(id),
 task_id VARCHAR(48) NOT NULL DEFAULT 'sum-two' REFERENCES tasks(id),
 winner_id UUID REFERENCES users(id),
 finished_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(player_one <> player_two)
);
ALTER TABLE submissions ADD COLUMN IF NOT EXISTS room_id UUID REFERENCES duel_rooms(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS submissions_room_idx ON submissions(room_id,created_at DESC);
CREATE TABLE IF NOT EXISTS duel_events (
 id BIGSERIAL PRIMARY KEY,
 room_id UUID NOT NULL REFERENCES duel_rooms(id) ON DELETE CASCADE,
 actor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 event_type VARCHAR(32) NOT NULL,
 payload JSONB NOT NULL DEFAULT '{}'::jsonb,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO tasks(id,title,category,difficulty,description,input_format,output_format,sample_input,sample_output,time_limit_ms,sort_order) VALUES
('sum-two','Сумма двух чисел','Основы','Лёгкая','Даны два целых числа. Выведите их сумму.','Два целых числа a и b, разделённые пробелом.','Одно целое число — a + b.','2 3','5',1000,1),
('max-three','Максимум из трёх','Основы','Лёгкая','Даны три целых числа. Найдите наибольшее.','Три целых числа, разделённые пробелом.','Максимальное из трёх чисел.','4 9 2','9',1000,2),
('reverse-string','Разворот строки','Строки','Средняя','Выведите символы входной строки в обратном порядке.','Одна строка текста.','Исходная строка в обратном порядке.','arena','anera',1000,3),
('balanced-brackets','Сбалансированные скобки','Стек','Средняя','Определите, корректно ли вложены круглые скобки.','Строка, содержащая только символы ( и ).','YES, если строка сбалансирована, иначе NO.','(()())','YES',1500,4),
('two-sum','Два слагаемых','Массивы','Сложная','Для массива и target найдите индексы двух различных элементов, сумма которых равна target. Гарантируется ровно одно решение.','Первая строка: n и target. Вторая строка: n целых чисел.','Два индекса с нумерацией с нуля.','4 9\n2 7 11 15','0 1',2000,5)
ON CONFLICT(id) DO NOTHING;
