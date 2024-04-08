package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestCheckIPInSgWithMultipleIpRanges(t *testing.T) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		t.Fatalf("unable to load SDK config, %v", err)
	}

	client := ec2.NewFromConfig(cfg)

	// Mock security group data
	mockIpPermissions := []*types.IpPermission{
		{
			IpRanges: []types.IpRange{
				{
					CidrIp: aws.String("192.168.1.0/24"),
				},
				{
					CidrIp: aws.String("10.0.0.0/16"),
				},
			},
			ToPort:   aws.Int32(8080),
			FromPort: aws.Int32(8080),
		},
		{
			IpRanges: []types.IpRange{
				{
					CidrIp: aws.String("172.16.0.0/20"),
				},
			},
			ToPort:   aws.Int32(80),
			FromPort: aws.Int32(80),
		},
	}

	// Create a mock security group
	mockSg := &types.SecurityGroup{
		IpPermissions: mockIpPermissions,
	}

	// Test cases for IPs within the specified ranges and matching ports
	if !CheckIPInSg(client, mockSg, "192.168.1.1", 8080) {
		t.Error("Expected IP to be in SecurityGroup")
	}
	if !CheckIPInSg(client, mockSg, "10.0.1.1", 8080) {
		t.Error("Expected IP to be in SecurityGroup")
	}
	if !CheckIPInSg(client, mockSg, "172.16.0.1", 80) {
		t.Error("Expected IP to be in SecurityGroup")
	}

	// Test cases for IPs outside the specified ranges
	if CheckIPInSg(client, mockSg, "192.168.2.1", 8080) {
		t.Error("Expected IP to not be in SecurityGroup")
	}
	if CheckIPInSg(client, mockSg, "10.1.1.1", 8080) {
		t.Error("Expected IP to not be in SecurityGroup")
	}
	if CheckIPInSg(client, mockSg, "172.16.32.1", 80) {
		t.Error("Expected IP to not be in SecurityGroup")
	}

	// Test cases for ports not matching
	if CheckIPInSg(client, mockSg, "192.168.1.1", 80) {
		t.Error("Expected port to not match")
	}
	if CheckIPInSg(client, mockSg, "10.0.1.1", 80) {
		t.Error("Expected port to not match")
	}
	if CheckIPInSg(client, mockSg, "172.16.0.1", 8080) {
		t.Error("Expected port to not match")
	}
}
