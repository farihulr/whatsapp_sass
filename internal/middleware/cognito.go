package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

type contextKey string

const (
	cognitoSubjectKey       contextKey = "cognito_subject"
	cognitoEmailKey         contextKey = "cognito_email"
	cognitoEmailVerifiedKey contextKey = "cognito_email_verified"
)

type CognitoMiddleware struct {
	keySet   jwk.Set
	issuer   string
	clientID string
}

func NewCognitoMiddleware(
	ctx context.Context,
	region string,
	userPoolID string,
	appClientID string,
) (*CognitoMiddleware, error) {
	if region == "" || userPoolID == "" || appClientID == "" {
		return nil, errors.New("konfigurasi Cognito belum lengkap")
	}

	issuer := fmt.Sprintf(
		"https://cognito-idp.%s.amazonaws.com/%s",
		region,
		userPoolID,
	)

	keySet, err := jwk.Fetch(ctx, issuer+"/.well-known/jwks.json")
	if err != nil {
		return nil, fmt.Errorf(
			"gagal mengambil public keys Cognito: %w",
			err,
		)
	}

	return &CognitoMiddleware{
		keySet:   keySet,
		issuer:   issuer,
		clientID: appClientID,
	}, nil
}

func (m *CognitoMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))

		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeAuthError(w, http.StatusUnauthorized, "Bearer token diperlukan")
			return
		}

		token, err := jwt.Parse(
			[]byte(parts[1]),
			jwt.WithKeySet(m.keySet),
			jwt.WithIssuer(m.issuer),
			jwt.WithClaimValue("token_use", "access"),
			jwt.WithClaimValue("client_id", m.clientID),
			jwt.WithRequiredClaim("exp"),
			jwt.WithRequiredClaim("iat"),
			jwt.WithAcceptableSkew(30*time.Second),
		)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "Access token tidak valid")
			return
		}

		subject := token.Subject()
		if subject == "" {
			writeAuthError(w, http.StatusUnauthorized, "Token tidak memiliki sub")
			return
		}

		ctx := context.WithValue(
			r.Context(),
			cognitoSubjectKey,
			subject,
		)

		if value, ok := token.Get("email"); ok {
			if email, ok := value.(string); ok && email != "" {
				ctx = context.WithValue(ctx, cognitoEmailKey, email)
			}
		}

		if value, ok := token.Get("email_verified"); ok {
			if verified, ok := value.(bool); ok {
				ctx = context.WithValue(
					ctx,
					cognitoEmailVerifiedKey,
					verified,
				)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CognitoSubject(ctx context.Context) (string, bool) {
	subject, ok := ctx.Value(cognitoSubjectKey).(string)
	return subject, ok && subject != ""
}

func CognitoEmail(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(cognitoEmailKey).(string)
	return email, ok && email != ""
}

func CognitoEmailVerified(ctx context.Context) bool {
	verified, _ := ctx.Value(cognitoEmailVerifiedKey).(bool)
	return verified
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
