package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-resty/resty/v2"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/multiformats/go-multihash"
	"github.com/wealdtech/go-ens/v3"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	rpcURL          = "https://api-gateway.skymavis.com/rpc?apikey="
	contractAddress = "0xadb077d236d9e81fb24b96ae9cb8089ab9942d48"
	contractABI     = `[{"inputs":[{"internalType":"bytes32","name":"node","type":"bytes32"}],"name":"contentHash","outputs":[{"internalType":"bytes","name":"","type":"bytes"}],"stateMutability":"view","type":"function"}]`
)

func main() {

	e := echo.New()

	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.RequestID())
	e.Use(middleware.Gzip())

	client, err := ethclient.Dial(rpcURL + os.Getenv("API_KEY"))
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	e.GET("/health", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	e.GET("/:address", func(c echo.Context) error {
		address := c.Param("address")

		if !strings.HasSuffix(address, ".ron") {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid address")
		}

		hash, err := getContentHash(client, address)

		if err != nil {
			log.Printf("Failed to get content hash: %+v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get content hash")
		}

		content, contentType, err := loadContent(*hash)

		if err != nil {
			log.Printf("Failed to load content: %+v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to load content")
		}

		if contentType == nil {
			return c.Blob(http.StatusOK, "application/octet-stream", []byte(*content))
		}

		if *contentType == "text/html" {
			return c.HTML(http.StatusOK, *content)
		}

		if *contentType == "text/plain" {
			return c.String(http.StatusOK, *content)
		}

		if strings.HasPrefix(*contentType, "image/") {
			return c.Blob(http.StatusOK, *contentType, []byte(*content))
		}

		return c.Blob(http.StatusOK, "application/octet-stream", []byte(*content))

	})

	e.GET("/hash/node/:address", func(c echo.Context) error {
		address := c.Param("address")
		hash, err := ens.NameHash(address)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid address")
		}

		return c.JSON(http.StatusOK, map[string]string{"hash": hex.EncodeToString(hash[:])})
	})

	e.GET("/hash/ipfs/:address", func(c echo.Context) error {
		address := c.Param("address")
		hash, err := multihash.FromB58String(address)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid hash")
		}

		return c.JSON(http.StatusOK, map[string]string{"hash": hash.HexString()})
	})

	e.Logger.Fatal(e.Start(fmt.Sprintf("0.0.0.0:%s", os.Getenv("PORT"))))

}

func getContentHash(client *ethclient.Client, name string) (*string, error) {
	contract := common.HexToAddress(contractAddress)
	parsedABI, err := abi.JSON(strings.NewReader(contractABI))
	if err != nil {
		return nil, err
	}

	// Assuming node is hardcoded or defined elsewhere as [32]byte
	node, err := ens.NameHash(name)

	if err != nil {
		return nil, err
	}

	data, err := parsedABI.Pack("contentHash", node)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{To: &contract, Data: data}
	result, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, err
	}

	// Parse result
	var contentHash []byte
	err = parsedABI.UnpackIntoInterface(&contentHash, "contentHash", result)
	if err != nil {
		return nil, err
	}

	_, out, err := multihash.MHFromBytes(contentHash)

	if err != nil {
		return nil, err
	}

	hash := out.B58String()
	return &hash, nil
}

func loadContent(hash string) (*string, *string, error) {

	r := resty.New()
	ctx := context.Background()
	ctxTimeout, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	var result string
	resp, err := r.R().SetContext(ctxTimeout).Get(fmt.Sprintf("%s/ipfs/%s", os.Getenv("IPFS_GATEWAY"), hash))

	if err != nil {
		return nil, nil, err
	}

	if resp.IsError() {
		if resp.Error() != nil {
			return nil, nil, resp.Error().(error)
		}
		return nil, nil, errors.New(resp.Status())
	}

	result = resp.String()

	contentType := resp.RawResponse.Header.Get("Content-Type")

	return &result, &contentType, nil
}
