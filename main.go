package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lpernett/godotenv"
)

var (
	consumerKey  string
	consumerSecret  string
	mpesaShortcode  string
	mpesaPassKey  string
	mpesaTokenUrl  string
	myEndpoint  string
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	consumerKey = os.Getenv("MPESA_CONSUMER_KEY")
	consumerSecret = os.Getenv("MPESA_CONSUMER_SECRET")
	mpesaShortcode = os.Getenv("MPESA_SHORTCODE")
	mpesaPassKey = os.Getenv("MPESA_PASS_KEY")
	mpesaTokenUrl = os.Getenv("MPESA_TOKEN_URL")
	myEndpoint = "https://webhook.site/9e1a6307-9adc-465b-a37b-78db245785a7"
	// myEndpoint = "https://spookie.requestcatcher.com"
	// myEndpoint = "https://sms-api.marps.co.ke"
}

func main() {
	router := gin.Default()

	router.Any("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Spookie's Mpesa Integration Service")
	})

	router.Any("/pay", MpesaExpress)
	router.POST("/callback", MpesaCallback)

	if err := router.Run(":3000"); err != nil {
		log.Fatal(err)
	}
}


func MpesaExpress(c *gin.Context) {
	var amount, phone string
	if c.Request.Method == http.MethodGet {
		amount = c.Query("amount")
		phone = c.Query("phone")
	} else if c.Request.Method == http.MethodPost {
		amount = c.PostForm("amount")
		phone = c.PostForm("phone")
	}

	if len(phone) == 0 || len(amount)== 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "phone and amount are required",
		})
		return
		}

	if phone[0] == '0' {
		phone = "254" + phone[1:]
	} else if phone[:3] != "254" {
		phone = "254" + phone
	}

	if _, err := strconv.Atoi(phone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Phone number must contain only digits"})
		return
	}

	if len(phone) < 9 || len(phone) > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Phone Number"}) 
		return
	}

	amountValue, err := strconv.ParseFloat(amount, 64)
	if err != nil || amountValue <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount must be greater than 0"})
		return
	}

	accessToken, err := getAccessToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get access token"})
		return
	}

	timestamp := time.Now().Format("20060102150405")
	password := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s%s%s", mpesaShortcode, mpesaPassKey, timestamp)))

	payload := map[string]interface{}{
		"BusinessShortCode": mpesaShortcode,
		"Password": password,
		"Timestamp": timestamp,
		"TransactionType": "CustomerPayBillOnline",
		"Amount": amount,
		"PartyA": phone,
		"PartyB": mpesaShortcode,
		"PhoneNumber": phone,
		"CallBackURL": myEndpoint + "/callback",
		// "CallBackUrl": myEndpoint,
		"AccountReference": "Marps Africa",
		"TransactionDesc": "Payment Testing",
	}

	payloadBytes, err := json.Marshal(payload)
	log.Printf("Sending payload: %+v\n", string(payloadBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal payload"})
		return
	}

	endpoint := "https://sandbox.safaricom.co.ke/mpesa/stkpush/v1/processrequest"

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Go-http-client/1.1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send request"})
		return
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{ "error": "Failed to read response body"})
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unmarshal response"})
		return
	}

	c.JSON(resp.StatusCode, result)

}

func MpesaCallback(c *gin.Context) {
    var callbackData struct {
        Body struct {
            StkCallback struct {
                MerchantRequestID string `json:"MerchantRequestID"`
                CheckoutRequestID string `json:"CheckoutRequestID"`
                ResultCode        int    `json:"ResultCode"`
                ResultDesc        string `json:"ResultDesc"`
                CallbackMetadata  struct {
                    Item []struct {
                        Name  string      `json:"Name"`
                        Value interface{} `json:"Value"`
                    } `json:"Item"`
                } `json:"CallbackMetadata"`
            } `json:"stkCallback"`
        } `json:"Body"`
    }

    if err := c.ShouldBindJSON(&callbackData); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
        return
    }

    log.Printf("Callback data received: %+v\n", callbackData)

    // Process successful transaction
    if callbackData.Body.StkCallback.ResultCode == 0 {
        // Extract values from callback metadata
        var amount float64
        var receiptNumber string
        var transactionDate int64
        var phoneNumber int64

        for _, item := range callbackData.Body.StkCallback.CallbackMetadata.Item {
            switch item.Name {
            case "Amount":
                amount, _ = item.Value.(float64)
            case "MpesaReceiptNumber":
                receiptNumber, _ = item.Value.(string)
            case "TransactionDate":
                transactionDate, _ = item.Value.(int64)
            case "PhoneNumber":
                phoneNumber, _ = item.Value.(int64)
            }
        }

		log.Printf("Amount: %v\n", amount)
		log.Printf("Receipt Number: %v\n", receiptNumber)
		log.Printf("Transaction Date: %v\n", transactionDate)
		log.Printf("Phone Number: %v\n", phoneNumber)

        // Convert transaction date to readable format
        dateStr := fmt.Sprintf("%d", transactionDate)
        parsedDate, _ := time.Parse("20060102150405", dateStr)
        formattedDate := parsedDate.Format("2006-01-02 15:04:05")
		
		log.Printf("Formatted Date: %v\n", formattedDate)

        // Save to database (pseudocode)
        // db.SaveTransaction(receiptNumber, amount, phoneNumber, formattedDate)

        // Send confirmation to customer (pseudocode)
        // sms.Send(phoneNumber, fmt.Sprintf("Payment of %v received. Receipt: %s", amount, receiptNumber))

        c.JSON(http.StatusOK, gin.H{
            "status": "success",
            "message": "Payment processed successfully",
            "receipt": receiptNumber,
        })
    } else {
        // Handle failed transaction
        c.JSON(http.StatusOK, gin.H{
            "status": "failed",
            "message": callbackData.Body.StkCallback.ResultDesc,
        })
    }
}

func getAccessToken() (string, error) {
	// log.Println("Getting access token...")
    req, err := http.NewRequest("GET", mpesaTokenUrl, nil)
    if err != nil {
        log.Printf("Failed to create request: %v\n", err)
        return "", err
    }

    req.SetBasicAuth(consumerKey, consumerSecret)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Failed to send request: %v\n", err)
        return "", err
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Failed to read response body: %v\n", err)
        return "", err
    }

    log.Printf("Response Status: %s\n", resp.Status)
    // log.Printf("Response Body: %s\n", string(body))

    var result map[string]interface{}
    if err := json.Unmarshal(body, &result); err != nil {
        log.Printf("Failed to unmarshal response: %v\n", err)
        return "", err
    }

    if accessToken, ok := result["access_token"].(string); ok {
        return accessToken, nil
    }

    log.Printf("Access token not found in response: %+v\n", result)
    return "", fmt.Errorf("access token not found in response")
}

