package queues

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/go-azure-sdk/sdk/client"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
	"github.com/jackofallops/giovanni/storage/internal/metadata"
)

type SetMetaDataResponse struct {
	HttpResponse *client.Response
}

type SetMetaDataInput struct {
	MetaData map[string]string
}

// SetMetaData returns the metadata for this Queue
func (c Client) SetMetaData(ctx context.Context, queueName string, input SetMetaDataInput) (resp SetMetaDataResponse, err error) {

	if queueName == "" {
		return resp, fmt.Errorf("`queueName` cannot be an empty string")
	}

	if strings.ToLower(queueName) != queueName {
		return resp, fmt.Errorf("`queueName` must be a lower-cased string")
	}

	if err := metadata.Validate(input.MetaData); err != nil {
		return resp, fmt.Errorf("`metadata` is not valid: %v", err)
	}

	opts := client.RequestOptions{
		ContentType: "application/xml; charset=utf-8",
		ExpectedStatusCodes: []int{
			http.StatusNoContent,
		},
		HttpMethod: http.MethodPut,
		OptionsObject: setMetaDataOptions{
			metadata: input.MetaData,
		},
		Path: fmt.Sprintf("/%s", queueName),
	}

	req, err := c.Client.NewRequest(ctx, opts)
	if err != nil {
		err = fmt.Errorf("building request: %+v", err)
		return
	}

	resp.HttpResponse, err = req.Execute(ctx)
	if err != nil {
		err = fmt.Errorf("executing request: %+v", err)
		return
	}

	return
}

type setMetaDataOptions struct {
	metadata map[string]string
}

func (s setMetaDataOptions) ToHeaders() *client.Headers {
	headers := &client.Headers{}
	headers.Merge(metadata.SetMetaDataHeaders(s.metadata))
	return headers
}

func (s setMetaDataOptions) ToOData() *odata.Query {
	return nil
}

func (s setMetaDataOptions) ToQuery() *client.QueryParams {
	out := &client.QueryParams{}
	out.Append("comp", "metadata")
	return out
}
