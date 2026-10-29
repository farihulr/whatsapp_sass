package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

type CognitoAuthService struct {
	client       *cognito.Client
	clientID     string
	clientSecret string
}

func NewCognitoAuthService(
	client *cognito.Client,
	clientID string,
	clientSecret string,
) *CognitoAuthService {
	return &CognitoAuthService{
		client:       client,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
	}
}

// secretHash membuat hash jika App Client Cognito menggunakan client secret.
func (s *CognitoAuthService) secretHash(username string) *string {
	if s.clientSecret == "" {
		return nil
	}

	mac := hmac.New(sha256.New, []byte(s.clientSecret))
	mac.Write([]byte(username + s.clientID))

	hash := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return aws.String(hash)
}

// Register mendaftarkan user baru ke AWS Cognito.
func (s *CognitoAuthService) Register(
	ctx context.Context,
	email string,
	name string,
	password string,
) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("Cognito client belum dikonfigurasi")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)

	if email == "" || name == "" || password == "" {
		return fmt.Errorf("email, nama, dan password wajib diisi")
	}

	if s.clientID == "" {
		return fmt.Errorf("Cognito App Client ID belum dikonfigurasi")
	}

	input := &cognito.SignUpInput{
		ClientId: aws.String(s.clientID),
		Username: aws.String(email),
		Password: aws.String(password),
		UserAttributes: []types.AttributeType{
			{
				Name:  aws.String("email"),
				Value: aws.String(email),
			},
			{
				Name:  aws.String("name"),
				Value: aws.String(name),
			},
		},
	}

	if hash := s.secretHash(email); hash != nil {
		input.SecretHash = hash
	}

	_, err := s.client.SignUp(ctx, input)
	if err != nil {
		return fmt.Errorf("registrasi Cognito gagal: %w", err)
	}

	return nil
}

// ConfirmRegistration memverifikasi kode yang dikirim Cognito ke email user.
func (s *CognitoAuthService) ConfirmRegistration(
	ctx context.Context,
	email string,
	code string,
) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("Cognito client belum dikonfigurasi")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)

	if email == "" || code == "" {
		return fmt.Errorf("email dan kode verifikasi wajib diisi")
	}

	if s.clientID == "" {
		return fmt.Errorf("Cognito App Client ID belum dikonfigurasi")
	}

	input := &cognito.ConfirmSignUpInput{
		ClientId:         aws.String(s.clientID),
		Username:         aws.String(email),
		ConfirmationCode: aws.String(code),
	}

	if hash := s.secretHash(email); hash != nil {
		input.SecretHash = hash
	}

	_, err := s.client.ConfirmSignUp(ctx, input)
	if err != nil {
		return fmt.Errorf("verifikasi Cognito gagal: %w", err)
	}

	return nil
}

func (s *CognitoAuthService) Login(
	ctx context.Context,
	email string,
	password string,
) (*cognito.InitiateAuthOutput, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("Cognito client belum dikonfigurasi")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, fmt.Errorf("email dan password wajib diisi")
	}
	if s.clientID == "" {
		return nil, fmt.Errorf("Cognito App Client ID belum dikonfigurasi")
	}

	input := &cognito.InitiateAuthInput{
		AuthFlow: types.AuthFlowTypeUserPasswordAuth,
		ClientId: aws.String(s.clientID),
		AuthParameters: map[string]string{
			"USERNAME": email,
			"PASSWORD": password,
		},
	}

	if hash := s.secretHash(email); hash != nil {
		input.AuthParameters["SECRET_HASH"] = *hash
	}

	result, err := s.client.InitiateAuth(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("login Cognito gagal: %w", err)
	}

	if result.AuthenticationResult == nil {
		return nil, fmt.Errorf("login memerlukan langkah autentikasi tambahan")
	}

	return result, nil
}
