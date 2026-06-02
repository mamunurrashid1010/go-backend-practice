# Day 18 — JWT: Issuing Access Tokens

> **Goal:** understand the three parts of a JWT (header, payload, signature), pick a signing algorithm, and change `POST /auth/login` so it returns a signed access token. **Verification + middleware comes Day 19** — today is only the *issue* half.

This builds on Day 17. Same `users` table, same register flow, same password verification — login now returns a token instead of just the user.

---

## 1. What is a JWT (in 30 seconds)

A **JWT** (JSON Web Token) is three Base64URL-encoded JSON blobs joined by dots:

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOjEsImV4cCI6MTcwMD...kc4.dGhlLXNpZ25hdHVyZQ
└────────── header ──────────┘└─────────────── payload ───────────────┘└──── signature ────┘
```

Decode the first two with any base64 tool and you'll see plain JSON. **A JWT is encoded, NOT encrypted.** Anyone can read the contents. The signature only proves the token wasn't tampered with.

Glue this together with the three parts:

```
header    = {"alg":"HS256","typ":"JWT"}
payload   = {"uid":1,"email":"a@b.com","exp":1700000000,"iat":1699999100,"iss":"day18-auth"}
signature = HMAC-SHA256( base64(header) + "." + base64(payload), SECRET )
token     = base64(header) + "." + base64(payload) + "." + base64(signature)
```

The server keeps `SECRET`. Without it, an attacker can't forge a valid signature, and even tweaking one character in the payload makes the signature wrong.

> **Try it now:** paste a JWT into <https://jwt.io> and watch the three parts decode in real time. Get used to that decoder — half of debugging JWT issues is "what's in the token *right now*?"

---

## 2. The three parts in detail

### Header

```json
{ "alg": "HS256", "typ": "JWT" }
```

Says which algorithm signed this token. The library reads this on verify and refuses anything you didn't allow.

> **`alg: none` is a real footgun.** Older JWT libraries accepted unsigned tokens by default. Always pin the expected algorithm; `golang-jwt/jwt/v5` defaults to safe behavior.

### Payload (the claims)

```json
{
  "iss": "day18-auth",
  "sub": "1",
  "uid": 1,
  "email": "a@b.com",
  "iat": 1699999100,
  "nbf": 1699999100,
  "exp": 1700000000
}
```

Two kinds of claims live here:

- **Registered claims** (standard, three letters) — the JWT spec assigns meaning:
  | Claim | Meaning |
  | --- | --- |
  | `iss` | Issuer (who minted it) |
  | `sub` | Subject (usually the user id, as a string) |
  | `aud` | Audience (intended recipient) |
  | `exp` | Expiration time (Unix seconds) |
  | `nbf` | Not before (token not valid yet) |
  | `iat` | Issued at |
  | `jti` | Unique token ID (for revocation) |

- **Custom claims** — anything else you put on it. We use `uid` (the user ID as an integer) and `email`. Keep these small and non-sensitive.

> Rule: **the payload is public.** Don't put a password, a secret token, or personally-identifiable data the client shouldn't see.

### Signature

For HS256 (the algorithm we use):

```
signature = HMAC-SHA256( base64url(header) + "." + base64url(payload), SECRET )
```

A symmetric MAC. Anyone with the secret can both sign and verify — fine for a single backend. If you have multiple backends and you don't want all of them to be able to *sign* (just *verify*), use **RS256** instead: the server signs with a private key and verifiers only need the public key.

| Algorithm | Type | When |
| --- | --- | --- |
| **HS256** | symmetric (HMAC-SHA256) | one backend signs and verifies. Simplest. |
| **RS256** | asymmetric (RSA + SHA-256) | one backend signs; many services verify with the public key |
| **EdDSA** | asymmetric (Ed25519) | modern, faster than RSA. Still less universally supported |

We use HS256 today. Day 67's microservices day is where RS256 makes sense.

---

## 3. Why JWT?

Compare to a classic server-side session:

| | Session (cookie + DB row) | JWT |
| --- | --- | --- |
| State | server-side (DB / Redis) | stateless (server keeps the secret only) |
| Each request needs | a DB lookup to validate | a signature check (in-memory CPU) |
| Revocation | delete the row | hard — token's valid until `exp` |
| Distribution | tied to one backend | usable across services |
| Size on wire | small (cookie id) | bigger (whole encoded payload) |

JWT wins on **scaling reads** and **inter-service auth**. Sessions win on **fine-grained revocation**. Many real systems do both: short-lived JWT access tokens + a server-side refresh token (Day 20).

---

## 4. Library choice — `golang-jwt/jwt/v5`

```powershell
go get github.com/golang-jwt/jwt/v5
```

The maintained fork of the old `dgrijalva/jwt-go`. Idiomatic Go API, sensible defaults, the right thing to use in 2026.

You define a claims struct that embeds `jwt.RegisteredClaims`, then call `NewWithClaims` + `SignedString`:

```go
type AccessTokenClaims struct {
    UserID int64  `json:"uid"`
    Email  string `json:"email"`
    jwt.RegisteredClaims
}

claims := AccessTokenClaims{
    UserID: u.ID,
    Email:  u.Email,
    RegisteredClaims: jwt.RegisteredClaims{
        Issuer:    "day18-auth",
        Subject:   strconv.FormatInt(u.ID, 10),
        IssuedAt:  jwt.NewNumericDate(now),
        NotBefore: jwt.NewNumericDate(now),
        ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
    },
}

t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
signed, err := t.SignedString([]byte(secret))
```

[internal/auth/token.go](internal/auth/token.go) wraps that in a `TokenIssuer` with config-driven TTL + issuer + secret.

---

## 5. Short-lived access tokens

Access tokens should be **short** — 5–60 minutes is the typical range. We default to **15 minutes**. Why short?

- If a token leaks (browser dev tools, a log line, an XSS), the attack window is bounded.
- You can't really "log out" a JWT (it's stateless). Short expiry is the soft revocation.
- The refresh token (Day 20) handles "stay logged in for weeks without re-entering the password".

---

## 6. The new login response

Day 17 returned the `User`. Today it returns the OAuth2-shaped token bundle:

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOjE...",
  "token_type":   "Bearer",
  "expires_in":   900
}
```

| Field | Meaning |
| --- | --- |
| `access_token` | the JWT |
| `token_type`   | always `"Bearer"` |
| `expires_in`   | seconds until `exp`. Client uses this to schedule a refresh |

That shape matches the OAuth2 spec, so any HTTP client that speaks OAuth2 already knows how to handle it.

> The token is **NOT** returned in a Set-Cookie header today — we put it in the JSON body. Day 19's clients (SPA, mobile, CLI) attach it as `Authorization: Bearer <token>`. Cookie-based delivery is a different security tradeoff (CSRF, etc.); we'll stick with the header pattern.

---

## 7. The `JWT_SECRET` discipline

Three rules, in order of how badly you get burned by violating them:

1. **At least 256 bits of entropy.** A real random secret, not "supersecret123". Generate one once:
   ```powershell
   # Git Bash / WSL / Linux / macOS:
   openssl rand -hex 32

   # PowerShell native:
   $b = New-Object byte[] 32
   [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
   [Convert]::ToHexString($b).ToLower()
   ```
2. **Never commit it.** `.env` is gitignored; `.env.example` has a placeholder only.
3. **Different per environment.** Dev / staging / prod each get their own. Rotating prod's secret invalidates every existing token — that's a feature (the nuclear "log everyone out" button).

The config package has `JWT_SECRET,required` — the app refuses to start without it.

---

## 8. What changed from Day 17

| File | Change |
| --- | --- |
| `internal/auth/token.go` | **NEW** — `TokenIssuer.Issue(User) (token, ttl, err)` |
| `internal/auth/service.go` | `Login` now returns a `LoginResult` (user + token + ttl) |
| `internal/auth/handler.go` | `login` writes the OAuth2-shaped JSON |
| `internal/config/config.go` | new `AuthConfig` (`JWT_SECRET`, `JWT_ACCESS_TTL`, `JWT_ISSUER`) |
| `main.go` | builds the `TokenIssuer` from config, hands it to `NewService` |
| `.env / .env.example` | add `JWT_SECRET`, `JWT_ACCESS_TTL`, `JWT_ISSUER` |

Register, password.go, the repository, the migrations — all unchanged from Day 17.

---

## 9. Run it

```powershell
cd Day_18_jwt_issue
docker compose up -d
go mod init day18
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get github.com/jackc/pgx/v5/pgconn
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go get github.com/go-playground/validator/v10
go get golang.org/x/crypto/bcrypt
go get github.com/golang-jwt/jwt/v5
go run .
```

```powershell
# register
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register

# login → token bundle
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login
# {"access_token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...","token_type":"Bearer","expires_in":900}
```

Copy the `access_token` into <https://jwt.io>. You'll see:
- header: `{"alg":"HS256","typ":"JWT"}`
- payload: your `uid`, `email`, `iss`, `sub`, `iat`, `nbf`, `exp`
- signature: the third part, marked "Invalid Signature" by jwt.io (because it doesn't know your secret)

Paste your `JWT_SECRET` into jwt.io's signature field → "Signature Verified".

---

## 10. What's next

**Day 19** — the **other half**: an auth middleware that reads `Authorization: Bearer <token>`, validates the signature + claims, extracts `userID` from `uid`, and puts it on `r.Context()` so downstream handlers know who's calling. The same context-passing pattern from Day 6 — different value carried.

Then Day 20 adds refresh tokens (long-lived, server-stored, rotated), and Day 21 ships a Notes API where each user only sees their own notes.
