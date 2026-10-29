
package service

import (
	"context"
	"fmt"
	"strings"

	cognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
)

type CognitoProfile struct {
	Sub           string
	Email         string
	Name          string
	EmailVerified bool
}

type CognitoProfileService struct {
	client *cognito.Client
}

func NewCognitoProfileService(client *cognito.Client) *CognitoProfileService {
	return &CognitoProfileService{
		client: client,
	}
}

func (s *CognitoProfileService) GetProfile(
	ctx context.Context,
	accessToken string,
) (*CognitoProfile, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("Cognito client belum dikonfigurasi")
	}

	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("access token kosong")
	}

	result, err := s.client.GetUser(ctx, &cognito.GetUserInput{
		AccessToken: &accessToken,
	})
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil profil Cognito: %w", err)
	}

	profile := &CognitoProfile{}

	for _, attr := range result.UserAttributes {
		if attr.Name == nil || attr.Value == nil {
			continue
		}

		switch *attr.Name {
		case "sub":
			profile.Sub = *attr.Value
		case "email":
			profile.Email = *attr.Value
		case "name":
			profile.Name = *attr.Value
		case "email_verified":
			profile.EmailVerified = strings.EqualFold(
				*attr.Value,
				"true",
			)
		}
	}

	if profile.Sub == "" {
		return nil, fmt.Errorf("atribut sub tidak ditemukan")
	}

	if profile.Email == "" {
		return nil, fmt.Errorf("atribut email tidak tersedia")
	}

	if strings.TrimSpace(profile.Name) == "" {
		profile.Name = profile.Email
	}

	return profile, nil
}
