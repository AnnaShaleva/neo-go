package block

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/CityOfZion/neo-go/pkg/io"
	"github.com/stretchr/testify/require"
)

func getDecodedBlock(t *testing.T, i int) *Block {
	data, err := getBlockData(i)
	require.NoError(t, err)

	b, err := hex.DecodeString(data["raw"].(string))
	require.NoError(t, err)

	block := &Block{}
	r := io.NewBinReaderFromBuf(b)
	block.DecodeBinary(r)
	require.NoError(t, r.Err)

	return block
}

func getBlockData(i int) (map[string]interface{}, error) {
	b, err := ioutil.ReadFile(fmt.Sprintf("../test_data/block_%d.json", i))
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, err
}