package ingestion

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "regexp"
    "strconv"
    "strings"
    "time"

    "github.com/jackc/pgx/v5"

    "nepse/terminal/api/internal/marketdata"
    nepse "nepse/terminal/api/internal/securityclass"
)