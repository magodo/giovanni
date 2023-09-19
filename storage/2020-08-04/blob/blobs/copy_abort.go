package blobs

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/go-azure-sdk/sdk/client"
	"github.com/hashicorp/go-azure-sdk/sdk/odata"
)

type AbortCopyInput struct {
	// The Copy ID which should be aborted
	CopyID string

	// The ID of the Lease
	// This must be specified if a Lease is present on the Blob, else a 403 is returned
	LeaseID *string
}

type CopyAbortResponse struct {
	HttpResponse *client.Response
}

// AbortCopy aborts a pending Copy Blob operation, and leaves a destination blob with zero length and full metadata.
func (c Client) AbortCopy(ctx context.Context, containerName, blobName string, input AbortCopyInput) (resp CopyAbortResponse, err error) {

	if containerName == "" {
		return resp, fmt.Errorf("`containerName` cannot be an empty string")
	}

	if strings.ToLower(containerName) != containerName {
		return resp, fmt.Errorf("`containerName` must be a lower-cased string")
	}

	if blobName == "" {
		return resp, fmt.Errorf("`blobName` cannot be an empty string")
	}

	if input.CopyID == "" {
		return resp, fmt.Errorf("`input.CopyID` cannot be an empty string")
	}

	opts := client.RequestOptions{
		ExpectedStatusCodes: []int{
			http.StatusNoContent,
		},
		HttpMethod: http.MethodPut,
		OptionsObject: copyAbortOptions{
			input: input,
		},
		Path: fmt.Sprintf("/%s/%s", containerName, blobName),
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

type copyAbortOptions struct {
	input AbortCopyInput
}

func (c copyAbortOptions) ToHeaders() *client.Headers {
	headers := &client.Headers{}
	headers.Append("x-ms-copy-action", "abort")
	if c.input.LeaseID != nil {
		headers.Append("x-ms-lease-id", *c.input.LeaseID)
	}

	return headers
}

func (c copyAbortOptions) ToOData() *odata.Query {
	return nil
}

func (c copyAbortOptions) ToQuery() *client.QueryParams {
	out := &client.QueryParams{}
	out.Append("comp", "copy")
	out.Append("copyid", c.input.CopyID)
	return out
}
