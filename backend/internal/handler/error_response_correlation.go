package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func addErrorCorrelationFields(c *gin.Context, payload gin.H) {
	response.AddErrorCorrelationFields(c, payload)
}
