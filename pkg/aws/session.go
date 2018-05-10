package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
)

// Session sets up an AWS session with the region automatically detected from
// the environment or the ec2 metadata service if running on ec2.
func Session(config *aws.Config) (*session.Session, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		Config:            *config,
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, err
	}

	if aws.StringValue(sess.Config.Region) == "" {
		// try to get region from metadata service
		metadata := ec2metadata.New(sess)
		region, err := metadata.Region()
		if err != nil {
			return nil, err
		}
		sess.Config.Region = aws.String(region)
	}

	return sess, nil
}
