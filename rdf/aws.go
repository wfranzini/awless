package rdf

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/google/badwolf/triple"
	"github.com/google/badwolf/triple/literal"
	"github.com/google/badwolf/triple/node"
	"github.com/google/badwolf/triple/predicate"
	"github.com/wallix/awless/api"
)

const (
	REGION   = "/region"
	VPC      = "/vpc"
	SUBNET   = "/subnet"
	INSTANCE = "/instance"
	USER     = "/user"
	ROLE     = "/role"
	GROUP    = "/group"
	POLICY   = "/policy"
)

var regionL, vpcL, subnetL, instanceL, userL, roleL, groupL, policyL *literal.Literal

func init() {
	var err error
	if regionL, err = literal.DefaultBuilder().Build(literal.Text, REGION); err != nil {
		panic(err)
	}
	if vpcL, err = literal.DefaultBuilder().Build(literal.Text, VPC); err != nil {
		panic(err)
	}
	if subnetL, err = literal.DefaultBuilder().Build(literal.Text, SUBNET); err != nil {
		panic(err)
	}
	if instanceL, err = literal.DefaultBuilder().Build(literal.Text, INSTANCE); err != nil {
		panic(err)
	}
	if userL, err = literal.DefaultBuilder().Build(literal.Text, USER); err != nil {
		panic(err)
	}
	if roleL, err = literal.DefaultBuilder().Build(literal.Text, ROLE); err != nil {
		panic(err)
	}
	if groupL, err = literal.DefaultBuilder().Build(literal.Text, GROUP); err != nil {
		panic(err)
	}
	if policyL, err = literal.DefaultBuilder().Build(literal.Text, POLICY); err != nil {
		panic(err)
	}
}

func BuildAwsAccessGraph(region string, access *api.AwsAccess) (*Graph, error) {
	triples, err := buildAccessRdfTriples(region, access)
	if err != nil {
		return nil, err
	}

	return NewGraphFromTriples(triples), nil
}

func BuildAwsInfraGraph(region string, infra *api.AwsInfra) (*Graph, error) {
	triples, err := buildInfraRdfTriples(region, infra)
	if err != nil {
		return nil, err
	}

	return NewGraphFromTriples(triples), nil
}

func buildAccessRdfTriples(region string, access *api.AwsAccess) ([]*triple.Triple, error) {
	var triples []*triple.Triple

	regionN, err := node.NewNodeFromStrings(REGION, region)
	if err != nil {
		return triples, err
	}

	t, err := triple.New(regionN, HasTypePredicate, triple.NewLiteralObject(regionL))
	if err != nil {
		return triples, err
	}
	triples = append(triples, t)

	usersIndex := make(map[string]*node.Node)
	for _, user := range access.Users {
		userId := aws.StringValue(user.UserId)
		n, err := addNode(USER, userId, user, &triples)
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)
		t, err = triple.New(regionN, ParentOfPredicate, triple.NewNodeObject(n))
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)

		usersIndex[userId] = n
	}

	rolesIndex := make(map[string]*node.Node)
	for _, role := range access.Roles {
		roleId := aws.StringValue(role.RoleId)
		n, err := addNode(ROLE, roleId, role, &triples)
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)
		t, err = triple.New(regionN, ParentOfPredicate, triple.NewNodeObject(n))
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)

		rolesIndex[roleId] = n
	}

	groupsIndex := make(map[string]*node.Node)
	for _, group := range access.Groups {
		groupId := aws.StringValue(group.GroupId)
		n, err := addNode(GROUP, groupId, group, &triples)
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)
		t, err = triple.New(regionN, ParentOfPredicate, triple.NewNodeObject(n))
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)

		groupsIndex[groupId] = n

		for _, userId := range access.UsersByGroup[groupId] {
			if usersIndex[userId] == nil {
				return triples, fmt.Errorf("group %s has user %s, but this user does not exist", groupId, userId)
			}
			t, err = triple.New(n, ParentOfPredicate, triple.NewNodeObject(usersIndex[userId]))
			if err != nil {
				return triples, err
			}
			triples = append(triples, t)
		}
	}

	for _, policy := range access.LocalPolicies {
		policyId := aws.StringValue(policy.PolicyId)
		n, err := addNode(POLICY, policyId, policy, &triples)
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)
		t, err = triple.New(regionN, ParentOfPredicate, triple.NewNodeObject(n))
		if err != nil {
			return triples, err
		}
		triples = append(triples, t)

		for _, userId := range access.UsersByLocalPolicies[policyId] {
			if usersIndex[userId] == nil {
				return triples, fmt.Errorf("policy %s has user %s, but this user does not exist", policyId, userId)
			}
			t, err := triple.New(n, ParentOfPredicate, triple.NewNodeObject(usersIndex[userId]))
			if err != nil {
				return triples, err
			}
			triples = append(triples, t)
		}

		for _, groupId := range access.GroupsByLocalPolicies[policyId] {
			if groupsIndex[groupId] == nil {
				return triples, fmt.Errorf("policy %s has user %s, but this user does not exist", policyId, groupId)
			}
			t, err := triple.New(n, ParentOfPredicate, triple.NewNodeObject(groupsIndex[groupId]))
			if err != nil {
				return triples, err
			}
			triples = append(triples, t)
		}

		for _, roleId := range access.RolesByLocalPolicies[policyId] {
			if rolesIndex[roleId] == nil {
				return triples, fmt.Errorf("policy %s has user %s, but this user does not exist", policyId, roleId)
			}
			t, err := triple.New(n, ParentOfPredicate, triple.NewNodeObject(rolesIndex[roleId]))
			if err != nil {
				return triples, err
			}
			triples = append(triples, t)
		}
	}

	return triples, nil
}

func buildInfraRdfTriples(region string, awsInfra *api.AwsInfra) (triples []*triple.Triple, err error) {
	var vpcNodes, subnetNodes []*node.Node

	regionN, err := node.NewNodeFromStrings(REGION, region)
	if err != nil {
		return triples, err
	}

	t, err := triple.New(regionN, HasTypePredicate, triple.NewLiteralObject(regionL))
	if err != nil {
		return triples, err
	}
	triples = append(triples, t)

	for _, vpc := range awsInfra.Vpcs {
		n, err := addNode(VPC, aws.StringValue(vpc.VpcId), vpc, &triples)
		if err != nil {
			return triples, err
		}
		vpcNodes = append(vpcNodes, n)
		t, err := triple.New(regionN, ParentOfPredicate, triple.NewNodeObject(n))
		if err != nil {
			return triples, fmt.Errorf("region %s", err)
		}
		triples = append(triples, t)
	}

	for _, subnet := range awsInfra.Subnets {
		n, err := addNode(SUBNET, aws.StringValue(subnet.SubnetId), subnet, &triples)
		if err != nil {
			return triples, fmt.Errorf("subnet %s", err)
		}

		subnetNodes = append(subnetNodes, n)

		vpcN := findNodeById(vpcNodes, aws.StringValue(subnet.VpcId))
		if vpcN != nil {
			t, err := triple.New(vpcN, ParentOfPredicate, triple.NewNodeObject(n))
			if err != nil {
				return triples, fmt.Errorf("vpc %s", err)
			}
			triples = append(triples, t)
		}
	}

	for _, instance := range awsInfra.Instances {
		n, err := addNode(INSTANCE, aws.StringValue(instance.InstanceId), instance, &triples)
		if err != nil {
			return triples, err
		}

		subnetN := findNodeById(subnetNodes, aws.StringValue(instance.SubnetId))

		if subnetN != nil {
			t, err := triple.New(subnetN, ParentOfPredicate, triple.NewNodeObject(n))
			if err != nil {
				return triples, fmt.Errorf("instances subnet %s", err)
			}
			triples = append(triples, t)
		}
	}

	return triples, nil
}

func findNodeById(nodes []*node.Node, id string) *node.Node {
	for _, n := range nodes {
		if id == n.ID().String() {
			return n
		}
	}
	return nil
}

type User struct {
	Id string `aws:"UserId"`
}

type Role struct {
	Id string `aws:"RoleId"`
}

type Group struct {
	Id string `aws:"GroupId"`
}

type Policy struct {
	Id string `aws:"PolicyId"`
}

type Vpc struct {
	Id string `aws:"VpcId"`
}

type Subnet struct {
	Id    string `aws:"SubnetId"`
	VpcId string `aws:"VpcId"`
}

type Instance struct {
	Id        string `aws:"InstanceId"`
	Type      string `aws:"InstanceType"`
	SubnetId  string `aws:"SubnetId"`
	VpcId     string `aws:"VpcId"`
	PublicIp  string `aws:"PublicIpAddress"`
	PrivateIp string `aws:"PrivateIpAddress"`
	ImageId   string `aws:"ImageId"`
}

func addNode(nodeType, id string, awsNode interface{}, triples *[]*triple.Triple) (*node.Node, error) {
	n, err := node.NewNodeFromStrings(nodeType, id)
	if err != nil {
		return nil, err
	}
	var lit *literal.Literal
	if lit, err = literal.DefaultBuilder().Build(literal.Text, nodeType); err != nil {
		return nil, err
	}
	t, err := triple.New(n, HasTypePredicate, triple.NewLiteralObject(lit))
	if err != nil {
		return nil, err
	}
	*triples = append(*triples, t)

	nodeV := reflect.ValueOf(awsNode).Elem()
	var propP *predicate.Predicate
	var destType reflect.Type
	switch nodeType {
	case VPC:
		destType = reflect.TypeOf(Vpc{})
	case SUBNET:
		destType = reflect.TypeOf(Subnet{})
	case INSTANCE:
		destType = reflect.TypeOf(Instance{})
	case USER:
		destType = reflect.TypeOf(User{})
	case ROLE:
		destType = reflect.TypeOf(Role{})
	case GROUP:
		destType = reflect.TypeOf(Group{})
	case POLICY:
		destType = reflect.TypeOf(Policy{})
	default:
		return nil, fmt.Errorf("type %s is not managed", nodeType)
	}

	for i := 0; i < destType.NumField(); i++ {
		if propP, err = predicate.NewImmutable(destType.Field(i).Name); err != nil {
			return nil, err
		}
		var propL *literal.Literal
		if awsTag, ok := destType.Field(i).Tag.Lookup("aws"); ok {
			sourceField := nodeV.FieldByName(awsTag)
			if sourceField.IsValid() {
				stringValue := aws.StringValue(sourceField.Interface().(*string))
				if stringValue != "" {
					if propL, err = literal.DefaultBuilder().Build(literal.Text, stringValue); err != nil {
						return nil, err
					}
					propT, err := triple.New(n, propP, triple.NewLiteralObject(propL))
					if err != nil {
						return nil, err
					}
					*triples = append(*triples, propT)
				}
			}
		}
	}

	return n, nil
}