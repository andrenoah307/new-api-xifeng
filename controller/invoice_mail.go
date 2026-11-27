package controller

import (
	"net/http"
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var invoiceArchiveSlots = make(chan struct{}, 2)

func invoiceMailActor(c *gin.Context) (*model.User, bool) {
	actor, err := model.AuthorizeInvoiceMail(c.GetInt("id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Forbidden"})
		return nil, false
	}
	return actor, true
}

func UploadInvoiceMailBatch(c *gin.Context) {
	actor, ok := invoiceMailActor(c)
	if !ok {
		return
	}
	select {
	case invoiceArchiveSlots <- struct{}{}:
		defer func() { <-invoiceArchiveSlots }()
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Upload is busy; try again"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.InvoiceArchiveMaxSize+1024*1024)
	err := c.Request.ParseMultipartForm(1024 * 1024)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice ZIP archive"})
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice ZIP archive"})
		return
	}
	defer file.Close()
	directory, err := os.MkdirTemp("", "invoice-mail-")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Invoice upload failed"})
		return
	}
	defer os.RemoveAll(directory)
	files, ignored, err := service.ExtractInvoiceArchive(file, header.Size, directory)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice ZIP archive"})
		return
	}
	preview, err := service.CreateInvoiceMailBatch(c.Request.Context(), actor.Id, files, ignored)
	if err != nil {
		common.SysError("invoice upload failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Invoice upload failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}

func ListInvoiceMailBatches(c *gin.Context) {
	actor, ok := invoiceMailActor(c)
	if !ok {
		return
	}
	var batches []model.InvoiceMailBatch
	query := model.DB.Select("id, owner_id, created_at, expires_at, cleaned_at, cancelled_at")
	if actor.Role != common.RoleRootUser {
		query = query.Where("owner_id = ?", actor.Id)
	}
	if err := query.Order("created_at DESC").Limit(20).Find(&batches).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Invoice batch unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": batches})
}

func CancelInvoiceMailBatch(c *gin.Context) {
	actor, ok := invoiceMailActor(c)
	if !ok {
		return
	}
	preview, err := service.CancelInvoiceMailBatch(c.Request.Context(), actor.Id, c.Param("batch"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invoice batch unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}

func GetInvoiceMailBatch(c *gin.Context) {
	actor, ok := invoiceMailActor(c)
	if !ok {
		return
	}
	preview, err := service.PreviewInvoiceMailBatch(actor.Id, c.Param("batch"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invoice batch unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}

func SendInvoiceMailBatchItem(c *gin.Context) {
	actor, ok := invoiceMailActor(c)
	if !ok {
		return
	}
	id, err := strconv.Atoi(c.Param("ticket"))
	var request struct {
		Review string `json:"review"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if err != nil || id <= 0 || c.ShouldBindJSON(&request) != nil || (request.Review != "" && request.Review != "delivered" && request.Review != "not_delivered") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice request"})
		return
	}
	if request.Review != "delivered" {
		preview, queueErr := service.EnqueueInvoiceMailBatch(c.Request.Context(), actor.Id, c.Param("batch"), id, request.Review)
		if queueErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invoice batch unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
		return
	}
	err = service.SendInvoiceMailBatchItem(c.Request.Context(), actor.Id, c.Param("batch"), id, request.Review)
	if err != nil {
		common.SysError("invoice delivery failed: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invoice delivery failed; refresh the batch before retrying"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func QueueInvoiceMailBatch(c *gin.Context) {
	actor, ok := invoiceMailActor(c)
	if !ok {
		return
	}
	preview, err := service.EnqueueInvoiceMailBatch(c.Request.Context(), actor.Id, c.Param("batch"), 0, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invoice batch unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}

func PauseInvoiceMailBatch(c *gin.Context) {
	actor, ok := invoiceMailActor(c)
	if !ok {
		return
	}
	preview, err := service.PauseInvoiceMailBatch(c.Request.Context(), actor.Id, c.Param("batch"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invoice batch unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}
