// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package terraformaws

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ava-labs/avalanche-cli/pkg/terraform"

	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func terraformAWSProviderName(region string) string {
	return fmt.Sprintf("aws.%s", region)
}

func terraformAWSKeyName(region string) string {
	return fmt.Sprintf("kp_%s", region)
}

func terraformEIPName(region string) string {
	return fmt.Sprintf("eip_%s", region)
}

func terraformInstanceName(region string) string {
	return fmt.Sprintf("%s_%s", constants.AWSNodeAnsiblePrefix, region)
}

// SetCloudCredentials sets AWS account credentials defined in .aws dir in user home dir
func SetCloudCredentials(rootBody *hclwrite.Body, awsProfile string, regions []string) error {
	for _, region := range regions {
		provider := rootBody.AppendNewBlock("provider", []string{"aws"})
		providerBody := provider.Body()
		providerBody.SetAttributeValue("region", cty.StringVal(region))
		providerBody.SetAttributeValue("alias", cty.StringVal(region))
		if awsProfile != constants.AWSDefaultCredential {
			providerBody.SetAttributeValue("profile", cty.StringVal(awsProfile))
		}
	}
	return nil
}

// addSecurityGroupRuleToSg is to add sg rule to new sg
func addSecurityGroupRuleToSg(securityGroupBody *hclwrite.Body, sgType, description, protocol, ip string, port int64) {
	inboundGroup := securityGroupBody.AppendNewBlock(sgType, []string{})
	inboundGroupBody := inboundGroup.Body()
	inboundGroupBody.SetAttributeValue("description", cty.StringVal(description))
	inboundGroupBody.SetAttributeValue("from_port", cty.NumberIntVal(port))
	inboundGroupBody.SetAttributeValue("to_port", cty.NumberIntVal(port))
	inboundGroupBody.SetAttributeValue("protocol", cty.StringVal(protocol))
	var ipList []cty.Value
	ipList = append(ipList, cty.StringVal(ip))
	inboundGroupBody.SetAttributeValue("cidr_blocks", cty.ListVal(ipList))
}

// addNewSecurityGroupRule is to add sg rule to existing sg
func addNewSecurityGroupRule(rootBody *hclwrite.Body, region, sgRuleName, sgID, sgType, protocol, ip string, port int64) {
	securityGroupRule := rootBody.AppendNewBlock("resource", []string{"aws_security_group_rule", sgRuleName})
	securityGroupRuleBody := securityGroupRule.Body()
	securityGroupRuleBody.SetAttributeValue("provider", cty.StringVal(terraformAWSProviderName(region)))
	securityGroupRuleBody.SetAttributeValue("type", cty.StringVal(sgType))
	securityGroupRuleBody.SetAttributeValue("from_port", cty.NumberIntVal(port))
	securityGroupRuleBody.SetAttributeValue("to_port", cty.NumberIntVal(port))
	securityGroupRuleBody.SetAttributeValue("protocol", cty.StringVal(protocol))
	var ipList []cty.Value
	ipList = append(ipList, cty.StringVal(ip))
	securityGroupRuleBody.SetAttributeValue("cidr_blocks", cty.ListVal(ipList))
	securityGroupRuleBody.SetAttributeValue("security_group_id", cty.StringVal(sgID))
}

// SetSecurityGroup whitelists the ip addresses allowed to ssh into cloud server
func SetSecurityGroup(rootBody *hclwrite.Body, region, ipAddress, securityGroupName string) {
	inputIPAddress := ipAddress + "/32"
	securityGroup := rootBody.AppendNewBlock("resource", []string{"aws_security_group", fmt.Sprintf("ssh_avax_sg_%s", region)})
	securityGroupBody := securityGroup.Body()
	securityGroupBody.SetAttributeValue("name", cty.StringVal(securityGroupName))
	securityGroupBody.SetAttributeValue("provider", cty.StringVal(terraformAWSProviderName(region)))
	securityGroupBody.SetAttributeValue("description", cty.StringVal("Allow SSH, AVAX HTTP outbound traffic"))

	// enable inbound access for ip address inputIPAddress in port 22
	addSecurityGroupRuleToSg(securityGroupBody, "ingress", "TCP", "tcp", inputIPAddress, constants.SSHTCPPort)
	// "0.0.0.0/0" is a must-have ip address value for inbound and outbound calls
	addSecurityGroupRuleToSg(securityGroupBody, "ingress", "AVAX HTTP", "tcp", "0.0.0.0/0", constants.AvalanchegoAPIPort)
	// enable inbound access for ip address inputIPAddress in port 9650
	addSecurityGroupRuleToSg(securityGroupBody, "ingress", "AVAX HTTP", "tcp", inputIPAddress, constants.AvalanchegoAPIPort)
	// "0.0.0.0/0" is a must-have ip address value for inbound and outbound calls
	addSecurityGroupRuleToSg(securityGroupBody, "ingress", "AVAX Staking", "tcp", "0.0.0.0/0", constants.AvalanchegoP2PPort)
	addSecurityGroupRuleToSg(securityGroupBody, "egress", "Outbound traffic", "-1", "0.0.0.0/0", constants.OutboundPort)
}

func SetSecurityGroupRule(rootBody *hclwrite.Body, region, ipAddress, sgID string, ipInTCP, ipInHTTP bool) {
	inputIPAddress := ipAddress + "/32"
	if !ipInTCP {
		sgRuleName := "ipTcp" + strings.ReplaceAll(ipAddress, ".", "")
		addNewSecurityGroupRule(rootBody, region, sgRuleName, sgID, "ingress", "tcp", inputIPAddress, constants.SSHTCPPort)
	}
	if !ipInHTTP {
		sgRuleName := "ipHttp" + strings.ReplaceAll(ipAddress, ".", "")
		addNewSecurityGroupRule(rootBody, region, sgRuleName, sgID, "ingress", "tcp", inputIPAddress, constants.AvalanchegoAPIPort)
	}
}

// SetElasticIPs attach elastic IP(s) to the associated ec2 instance(s)
func SetElasticIPs(rootBody *hclwrite.Body, region string, numNodes int) {
	eip := rootBody.AppendNewBlock("resource", []string{"aws_eip", terraformEIPName(region)})
	eipBody := eip.Body()
	eipBody.SetAttributeValue("provider", cty.StringVal(terraformAWSProviderName(region)))
	eipBody.SetAttributeValue("count", cty.NumberIntVal(int64(numNodes)))
	eipBody.SetAttributeTraversal("instance", hcl.Traversal{
		hcl.TraverseRoot{
			Name: "aws_instance",
		},
		hcl.TraverseAttr{
			Name: fmt.Sprintf("%s[count.index]", terraformInstanceName(region)),
		},
		hcl.TraverseAttr{
			Name: "id",
		},
	})
	eipBody.SetAttributeValue("vpc", cty.BoolVal(true))
}

// SetKeyPair define the key pair that we will create in our EC2 instance if it doesn't exist yet and download the .pem file to home dir
func SetKeyPair(rootBody *hclwrite.Body, region, keyName, certName string) {
	// define the encryption we are using for the key pair
	tlsPrivateKey := rootBody.AppendNewBlock("resource", []string{"tls_private_key", "pk"})
	tlsPrivateKeyBody := tlsPrivateKey.Body()
	tlsPrivateKeyBody.SetAttributeValue("algorithm", cty.StringVal("RSA"))
	tlsPrivateKeyBody.SetAttributeValue("rsa_bits", cty.NumberIntVal(4096))

	// define the encryption we are using for the key pair
	keyPair := rootBody.AppendNewBlock("resource", []string{"aws_key_pair", terraformAWSKeyName(region)})
	keyPairBody := keyPair.Body()
	keyPairBody.SetAttributeValue("provider", cty.StringVal(terraformAWSProviderName(region)))
	keyPairBody.SetAttributeValue("key_name", cty.StringVal(keyName))
	keyPairBody.SetAttributeTraversal("public_key", hcl.Traversal{
		hcl.TraverseRoot{
			Name: "tls_private_key",
		},
		hcl.TraverseAttr{
			Name: "pk",
		},
		hcl.TraverseAttr{
			Name: "public_key_openssh",
		},
	})

	tfKey := rootBody.AppendNewBlock("resource", []string{"local_file", fmt.Sprintf("tf_key_%s", region)})
	tfKeyBody := tfKey.Body()
	tfKeyBody.SetAttributeValue("filename", cty.StringVal(certName))
	tfKeyBody.SetAttributeTraversal("content", hcl.Traversal{
		hcl.TraverseRoot{
			Name: "tls_private_key",
		},
		hcl.TraverseAttr{
			Name: terraformAWSKeyName(region),
		},
		hcl.TraverseAttr{
			Name: "private_key_pem",
		},
	})
}

// SetupInstances adds aws_instance section in terraform state file where we configure all the necessary components of the desired ec2 instance(s)
func SetupInstances(rootBody *hclwrite.Body, region, securityGroupName string, useExistingKeyPair bool, existingKeyPairName, ami string, numNodes int, instanceType string) {
	awsInstance := rootBody.AppendNewBlock("resource", []string{"aws_instance", terraformInstanceName(region)})
	awsInstanceBody := awsInstance.Body()
	awsInstanceBody.SetAttributeValue("count", cty.NumberIntVal(int64(numNodes)))
	awsInstanceBody.SetAttributeValue("ami", cty.StringVal(ami))
	awsInstanceBody.SetAttributeValue("instance_type", cty.StringVal(instanceType))
	if !useExistingKeyPair {
		awsInstanceBody.SetAttributeTraversal("key_name", hcl.Traversal{
			hcl.TraverseRoot{
				Name: "aws_key_pair",
			},
			hcl.TraverseAttr{
				Name: "kp",
			},
			hcl.TraverseAttr{
				Name: "key_name",
			},
		})
	} else {
		awsInstanceBody.SetAttributeValue("key_name", cty.StringVal(existingKeyPairName))
	}
	var securityGroupList []cty.Value
	securityGroupList = append(securityGroupList, cty.StringVal(securityGroupName))
	awsInstanceBody.SetAttributeValue("security_groups", cty.ListVal(securityGroupList))
	rootBlockDevice := awsInstanceBody.AppendNewBlock("root_block_device", []string{})
	rootBlockDeviceBody := rootBlockDevice.Body()
	rootBlockDeviceBody.SetAttributeValue("volume_size", cty.NumberIntVal(constants.CloudServerStorageSize))
}

// SetOutput adds output section in terraform state file so that we can call terraform output command and print instance_ip and instance_id to user
func SetOutput(rootBody *hclwrite.Body, regions []string, useEIP bool) {
	for _, region := range regions {
		if useEIP {
			outputEip := rootBody.AppendNewBlock("output", []string{"instance_ips_" + region})
			outputEipBody := outputEip.Body()
			outputEipBody.SetAttributeTraversal("value", hcl.Traversal{
				hcl.TraverseRoot{
					Name: "aws_eip",
				},
				hcl.TraverseAttr{
					Name: fmt.Sprintf("%s[*]", terraformEIPName(region)),
				},
				hcl.TraverseAttr{
					Name: "public_ip",
				},
			})
		}
		outputInstanceID := rootBody.AppendNewBlock("output", []string{"instance_ids_" + region})
		outputInstanceIDBody := outputInstanceID.Body()
		outputInstanceIDBody.SetAttributeTraversal("value", hcl.Traversal{
			hcl.TraverseRoot{
				Name: "aws_instance",
			},
			hcl.TraverseAttr{
				Name: fmt.Sprintf("%s[*]", terraformInstanceName(region)),
			},
			hcl.TraverseAttr{
				Name: "id",
			},
		})
		outputInstanceRegions := rootBody.AppendNewBlock("output", []string{"instance_regions_" + region})
		outputInstanceRegionsBody := outputInstanceRegions.Body()
		outputInstanceRegionsBody.SetAttributeTraversal("value", hcl.Traversal{
			hcl.TraverseRoot{
				Name: "aws_instance",
			},
			hcl.TraverseAttr{
				Name: fmt.Sprintf("%s[*]", terraformInstanceName(region)),
			},
			hcl.TraverseAttr{
				Name: "provider",
			},
		})
	}
}

// RunTerraform executes terraform apply function that creates the EC2 instances based on the .tf file provided
// returns a list of AWS node-IDs and node IPs
func RunTerraform(terraformDir string, regions []string, useEIP bool) (map[string][]string, map[string][]string, error) {
	var stdBuffer bytes.Buffer
	var stderr bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)
	cmd := exec.Command(constants.Terraform, "init") //nolint:gosec
	cmd.Dir = terraformDir
	cmd.Stdout = mw
	if err := cmd.Run(); err != nil {
		return nil, nil, err
	}
	cmd = exec.Command(constants.Terraform, "apply", "-auto-approve") //nolint:gosec
	cmd.Env = os.Environ()
	cmd.Dir = terraformDir
	cmd.Stdout = mw
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), constants.EIPLimitErr) {
			return nil, nil, errors.New(constants.EIPLimitErr)
		}
		return nil, nil, err
	}
	instanceIDs, err := GetInstanceIDs(terraformDir, regions)
	if err != nil {
		return nil, nil, err
	}
	publicIPs := map[string][]string{}
	if useEIP {
		publicIPs, err = terraform.GetPublicIPs(terraformDir, regions)
		if err != nil {
			return nil, nil, err
		}
	}
	return instanceIDs, publicIPs, nil
}

func GetInstanceIDs(terraformDir string, regions []string) (map[string][]string, error) {
	instanceIDs := map[string][]string{}
	for _, region := range regions {
		cmd := exec.Command(constants.Terraform, "output", "instance_ids_"+region) //nolint:gosec
		cmd.Dir = terraformDir
		instanceIDsOutput, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		instanceIDsOutputWoSpace := strings.TrimSpace(string(instanceIDsOutput))
		// eip and nodeID outputs are bounded by [ and ,] , we need to remove them
		trimmedInstanceIDs := instanceIDsOutputWoSpace[1 : len(instanceIDsOutputWoSpace)-3]
		splitInstanceIDs := strings.Split(trimmedInstanceIDs, ",")
		instanceIDs[region] = []string{}
		for _, instanceID := range splitInstanceIDs {
			instanceIDWoSpace := strings.TrimSpace(instanceID)
			// eip and nodeID both are bounded by double quotation "", we need to remove them before they can be used
			instanceIDs[region] = append(instanceIDs[region], instanceIDWoSpace[1:len(instanceIDWoSpace)-1])
		}
	}
	return instanceIDs, nil
}

func GetInstanceRegions(terraformDir string) ([]string, error) {
	cmd := exec.Command(constants.Terraform, "output", "instance_regions") //nolint:gosec
	cmd.Dir = terraformDir
	instanceRegionsOutput, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	instanceRegions := []string{}
	instanceRegionsOutputWoSpace := strings.TrimSpace(string(instanceRegionsOutput))
	// eip and nodeID outputs are bounded by [ and ,] , we need to remove them
	trimmedInstanceRegions := instanceRegionsOutputWoSpace[1 : len(instanceRegionsOutputWoSpace)-3]
	splitInstanceIDs := strings.Split(trimmedInstanceRegions, ",")
	for _, instanceID := range splitInstanceIDs {
		instanceRegionsWoSpace := strings.TrimSpace(instanceID)
		// eip and nodeID both are bounded by double quotation "", we need to remove them before they can be used
		instanceRegions = append(instanceRegions, strings.TrimPrefix(instanceRegionsWoSpace[1:len(instanceRegionsWoSpace)-1], "aws."))
	}
	return instanceRegions, nil
}
