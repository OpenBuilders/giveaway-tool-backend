-- Создание таблиц для Giveaway Tool

-- Таблица пользователей
CREATE TABLE IF NOT EXISTS users (
    id BIGINT PRIMARY KEY,
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    avatar_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица гивов
CREATE TABLE IF NOT EXISTS giveaways (
    id VARCHAR(255) PRIMARY KEY,
    creator_id BIGINT NOT NULL REFERENCES users(id),
    title VARCHAR(255) NOT NULL,
    description TEXT,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL,
    ends_at TIMESTAMP WITH TIME ZONE NOT NULL,
    duration BIGINT NOT NULL, -- в секундах
    max_participants INTEGER DEFAULT 0, -- 0 = безлимит
    winners_count INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    auto_distribute BOOLEAN DEFAULT false,
    allow_tickets BOOLEAN DEFAULT false,
    msg_id BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT valid_status CHECK (status IN ('active', 'pending', 'processing', 'custom', 'completed', 'history', 'cancelled')),
    CONSTRAINT valid_winners_count CHECK (winners_count > 0),
    CONSTRAINT valid_duration CHECK (duration > 0)
);

-- Таблица участников
CREATE TABLE IF NOT EXISTS participants (
    giveaway_id VARCHAR(255) NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (giveaway_id, user_id)
);

-- Таблица призов
CREATE TABLE IF NOT EXISTS prizes (
    id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    is_internal BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица призов гива (связь many-to-many)
CREATE TABLE IF NOT EXISTS giveaway_prizes (
    giveaway_id VARCHAR(255) NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    prize_id VARCHAR(255) NOT NULL REFERENCES prizes(id),
    place INTEGER NOT NULL, -- место приза (1, 2, 3, ...)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (giveaway_id, prize_id)
);

-- Таблица записей о победах
CREATE TABLE IF NOT EXISTS win_records (
    id VARCHAR(255) PRIMARY KEY,
    giveaway_id VARCHAR(255) NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    prize_id VARCHAR(255) REFERENCES prizes(id),
    place INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    received_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT valid_win_status CHECK (status IN ('pending', 'distributed', 'cancelled'))
);

-- Таблица требований
CREATE TABLE IF NOT EXISTS requirements (
    id SERIAL PRIMARY KEY,
    giveaway_id VARCHAR(255) NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    username VARCHAR(255),
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    CONSTRAINT valid_requirement_type CHECK (type IN ('subscription', 'boost', 'custom'))
);

-- Таблица спонсоров (каналов)
CREATE TABLE IF NOT EXISTS sponsors (
    id BIGINT PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    title VARCHAR(255),
    avatar_url TEXT,
    channel_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица спонсоров гива (связь many-to-many)
CREATE TABLE IF NOT EXISTS giveaway_sponsors (
    giveaway_id VARCHAR(255) NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    sponsor_id BIGINT NOT NULL REFERENCES sponsors(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (giveaway_id, sponsor_id)
);

-- Таблица билетов
CREATE TABLE IF NOT EXISTS tickets (
    giveaway_id VARCHAR(255) NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (giveaway_id, user_id)
);

-- Таблица pre-winner lists для custom требований
CREATE TABLE IF NOT EXISTS pre_winner_lists (
    giveaway_id VARCHAR(255) PRIMARY KEY REFERENCES giveaways(id) ON DELETE CASCADE,
    user_ids BIGINT[] NOT NULL, -- массив user_id
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL -- TTL для автоматической очистки
);

-- Индексы для производительности
CREATE INDEX IF NOT EXISTS idx_giveaways_creator_id ON giveaways(creator_id);
CREATE INDEX IF NOT EXISTS idx_giveaways_status ON giveaways(status);
CREATE INDEX IF NOT EXISTS idx_giveaways_ends_at ON giveaways(ends_at);
CREATE INDEX IF NOT EXISTS idx_participants_giveaway_id ON participants(giveaway_id);
CREATE INDEX IF NOT EXISTS idx_participants_user_id ON participants(user_id);
CREATE INDEX IF NOT EXISTS idx_win_records_giveaway_id ON win_records(giveaway_id);
CREATE INDEX IF NOT EXISTS idx_win_records_user_id ON win_records(user_id);
CREATE INDEX IF NOT EXISTS idx_requirements_giveaway_id ON requirements(giveaway_id);
CREATE INDEX IF NOT EXISTS idx_tickets_giveaway_id ON tickets(giveaway_id);
CREATE INDEX IF NOT EXISTS idx_tickets_user_id ON tickets(user_id);

-- Индекс для поиска активных гивов
CREATE INDEX IF NOT EXISTS idx_giveaways_active ON giveaways(status, ends_at) 
WHERE status IN ('active', 'pending', 'processing', 'custom');

-- Индекс для поиска завершенных гивов
CREATE INDEX IF NOT EXISTS idx_giveaways_completed ON giveaways(status, updated_at) 
WHERE status IN ('completed', 'history', 'cancelled');

-- Функция для автоматического обновления updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Триггеры для автоматического обновления updated_at
CREATE TRIGGER update_giveaways_updated_at BEFORE UPDATE ON giveaways
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_win_records_updated_at BEFORE UPDATE ON win_records
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_tickets_updated_at BEFORE UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Функция для очистки истекших pre-winner lists
CREATE OR REPLACE FUNCTION cleanup_expired_pre_winner_lists()
RETURNS void AS $$
BEGIN
    DELETE FROM pre_winner_lists WHERE expires_at < NOW();
END;
$$ language 'plpgsql';

-- Создаем задачу для автоматической очистки (если используется pg_cron)
-- SELECT cron.schedule('cleanup-pre-winner-lists', '0 */6 * * *', 'SELECT cleanup_expired_pre_winner_lists();'); 