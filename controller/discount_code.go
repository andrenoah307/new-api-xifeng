package controller

import (
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetAllDiscountCodes(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	codes, total, err := model.GetAllDiscountCodes(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

func SearchDiscountCodes(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	codes, total, err := model.SearchDiscountCodes(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(codes)
	common.ApiSuccess(c, pageInfo)
}

func GetDiscountCode(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	dc, err := model.GetDiscountCodeById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dc,
	})
}

func AddDiscountCode(c *gin.Context) {
	dc := model.DiscountCode{}
	err := c.ShouldBindJSON(&dc)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if dc.DiscountRate < 1 || dc.DiscountRate > 99 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "折扣率必须在 1-99 之间"})
		return
	}
	if dc.Count <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "数量必须大于 0"})
		return
	}
	if dc.Count > 100 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "单次最多创建 100 个"})
		return
	}
	if dc.EndTime > 0 && dc.EndTime < common.GetTimestamp() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "结束时间不能早于当前时间"})
		return
	}
	if dc.StartTime > 0 && dc.EndTime > 0 && dc.StartTime >= dc.EndTime {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "开始时间不能晚于或等于结束时间"})
		return
	}
	nameLen := utf8.RuneCountInString(dc.Name)
	if nameLen > 100 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "名称不能超过 100 个字符"})
		return
	}

	var codes []string
	for i := 0; i < dc.Count; i++ {
		code := dc.Code
		if code == "" {
			code = common.GetRandomString(32)
		} else if dc.Count > 1 {
			code = dc.Code + "-" + common.GetRandomString(8)
		}
		cleanDC := model.DiscountCode{
			Code:           code,
			Name:           dc.Name,
			DiscountRate:   dc.DiscountRate,
			StartTime:      dc.StartTime,
			EndTime:        dc.EndTime,
			MaxUsesTotal:   dc.MaxUsesTotal,
			MaxUsesPerUser: dc.MaxUsesPerUser,
			CreatedTime:    common.GetTimestamp(),
		}
		err = cleanDC.Insert()
		if err != nil {
			common.SysError("failed to insert discount code: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "创建折扣码失败",
				"data":    codes,
			})
			return
		}
		codes = append(codes, code)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    codes,
	})
}

func DeleteDiscountCode(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteDiscountCodeById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateDiscountCode(c *gin.Context) {
	statusOnly := c.Query("status_only")
	dc := model.DiscountCode{}
	err := c.ShouldBindJSON(&dc)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	cleanDC, err := model.GetDiscountCodeById(dc.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		if dc.DiscountRate < 1 || dc.DiscountRate > 99 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "折扣率必须在 1-99 之间"})
			return
		}
		if dc.EndTime > 0 && dc.EndTime < common.GetTimestamp() {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "结束时间不能早于当前时间"})
			return
		}
		if dc.Code != "" && dc.Code != cleanDC.Code {
			existing, _ := model.GetDiscountCodeByCode(dc.Code)
			if existing != nil && existing.Id != cleanDC.Id {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "折扣码已存在"})
				return
			}
			cleanDC.Code = dc.Code
		}
		cleanDC.Name = dc.Name
		cleanDC.DiscountRate = dc.DiscountRate
		cleanDC.StartTime = dc.StartTime
		cleanDC.EndTime = dc.EndTime
		cleanDC.MaxUsesTotal = dc.MaxUsesTotal
		cleanDC.MaxUsesPerUser = dc.MaxUsesPerUser
	}
	if statusOnly != "" {
		cleanDC.Status = dc.Status
	}
	err = cleanDC.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanDC,
	})
}

func ValidateUserDiscountCode(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
		return
	}
	userId := c.GetInt("id")
	dc, err := model.ValidateDiscountCode(req.Code, userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"discount_rate": dc.DiscountRate,
			"code":          dc.Code,
		},
	})
}

func CleanupDiscountCodePendingOrders(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的折扣码ID"})
		return
	}
	cleaned, err := model.CleanupPendingOrdersByDiscountCode(id, 30)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleaned,
	})
}
