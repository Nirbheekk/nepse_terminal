-- 0006_chart_layouts: user saved chart layouts and indicator configurations.
CREATE TABLE chart_layouts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
    name        text NOT NULL,
    symbol      text NOT NULL,
    interval    text NOT NULL DEFAULT '1d',
    chart_type  text NOT NULL DEFAULT 'candlestick',
    indicators  jsonb NOT NULL DEFAULT '[]'::jsonb,
    settings    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX chart_layouts_user_idx ON chart_layouts (user_id, updated_at DESC);
