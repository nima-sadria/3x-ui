package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/mieru"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// MieruController exposes CRUD and lifecycle endpoints for Mieru inbounds and
// users under /panel/api/mieru/. It is only registered when
// ENABLE_MIERU_PROVIDER=true.
type MieruController struct {
	mieruService service.MieruService
}

func NewMieruController(g *gin.RouterGroup) *MieruController {
	a := &MieruController{}
	a.initRouter(g)
	return a
}

func (a *MieruController) initRouter(g *gin.RouterGroup) {
	// Inbound management
	g.GET("/inbounds", a.listInbounds)
	g.GET("/inbounds/:id", a.getInbound)
	g.POST("/inbounds/add", a.addInbound)
	g.POST("/inbounds/update/:id", a.updateInbound)
	g.POST("/inbounds/del/:id", a.delInbound)
	g.POST("/inbounds/setEnable/:id", a.setInboundEnable)
	g.POST("/inbounds/apply/:id", a.applyInbound)
	g.POST("/inbounds/applyAll", a.applyAll)

	// User management
	g.GET("/users/:inboundId", a.listUsers)
	g.GET("/user/:id", a.getUser)
	g.POST("/users/add", a.addUser)
	g.POST("/users/update/:id", a.updateUser)
	g.POST("/users/del/:id", a.delUser)
	g.POST("/users/setEnable/:id", a.setUserEnable)

	// Client export
	g.GET("/users/export/:inboundId/:userId", a.exportClientJSON)
	g.GET("/users/exportText/:inboundId/:userId", a.exportClientText)

	// Mita process lifecycle
	g.GET("/status", a.status)
	g.POST("/start", a.start)
	g.POST("/stop", a.stop)
}

// ── Inbound handlers ──────────────────────────────────────────────────────────

func (a *MieruController) listInbounds(c *gin.Context) {
	rows, err := a.mieruService.ListInbounds()
	if err != nil {
		jsonMsg(c, "list mieru inbounds", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *MieruController) getInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	row, err := a.mieruService.GetInbound(id)
	if err != nil {
		jsonMsg(c, "get mieru inbound", err)
		return
	}
	jsonObj(c, row, nil)
}

func (a *MieruController) addInbound(c *gin.Context) {
	var ib model.MieruInbound
	if err := c.ShouldBindJSON(&ib); err != nil {
		jsonMsg(c, "parse mieru inbound", err)
		return
	}
	if err := a.mieruService.CreateInbound(&ib); err != nil {
		jsonMsg(c, "create mieru inbound", err)
		return
	}
	jsonObj(c, ib, nil)
}

func (a *MieruController) updateInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	var ib model.MieruInbound
	if err := c.ShouldBindJSON(&ib); err != nil {
		jsonMsg(c, "parse mieru inbound", err)
		return
	}
	ib.Id = id
	if err := a.mieruService.UpdateInbound(&ib); err != nil {
		jsonMsg(c, "update mieru inbound", err)
		return
	}
	jsonObj(c, ib, nil)
}

func (a *MieruController) delInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	if err := a.mieruService.DeleteInbound(id); err != nil {
		jsonMsg(c, "delete mieru inbound", err)
		return
	}
	jsonMsg(c, "deleted", nil)
}

func (a *MieruController) setInboundEnable(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	var body struct {
		Enable bool `json:"enable"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, "parse enable", err)
		return
	}
	if err := a.mieruService.SetInboundEnable(id, body.Enable); err != nil {
		jsonMsg(c, "set inbound enable", err)
		return
	}
	jsonMsg(c, "updated", nil)
}

func (a *MieruController) applyInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	if err := a.mieruService.ApplyInboundConfig(id); err != nil {
		jsonMsg(c, "apply mieru config", err)
		return
	}
	jsonMsg(c, "applied", nil)
}

func (a *MieruController) applyAll(c *gin.Context) {
	if err := a.mieruService.ApplyAllEnabledInbounds(); err != nil {
		jsonMsg(c, "apply all mieru inbounds", err)
		return
	}
	jsonMsg(c, "applied", nil)
}

// ── User handlers ─────────────────────────────────────────────────────────────

func (a *MieruController) listUsers(c *gin.Context) {
	inboundID, err := strconv.Atoi(c.Param("inboundId"))
	if err != nil {
		jsonMsg(c, "parse inboundId", err)
		return
	}
	rows, err := a.mieruService.ListUsers(inboundID)
	if err != nil {
		jsonMsg(c, "list mieru users", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *MieruController) getUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	u, err := a.mieruService.GetUser(id)
	if err != nil {
		jsonMsg(c, "get mieru user", err)
		return
	}
	jsonObj(c, u, nil)
}

func (a *MieruController) addUser(c *gin.Context) {
	var u model.MieruUser
	if err := c.ShouldBindJSON(&u); err != nil {
		jsonMsg(c, "parse mieru user", err)
		return
	}
	if err := a.mieruService.CreateUser(&u); err != nil {
		jsonMsg(c, "create mieru user", err)
		return
	}
	jsonObj(c, u, nil)
}

func (a *MieruController) updateUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	var u model.MieruUser
	if err := c.ShouldBindJSON(&u); err != nil {
		jsonMsg(c, "parse mieru user", err)
		return
	}
	u.Id = id
	if err := a.mieruService.UpdateUser(&u); err != nil {
		jsonMsg(c, "update mieru user", err)
		return
	}
	jsonObj(c, u, nil)
}

func (a *MieruController) delUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	if err := a.mieruService.DeleteUser(id); err != nil {
		jsonMsg(c, "delete mieru user", err)
		return
	}
	jsonMsg(c, "deleted", nil)
}

func (a *MieruController) setUserEnable(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "parse id", err)
		return
	}
	var body struct {
		Enable bool `json:"enable"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, "parse enable", err)
		return
	}
	if err := a.mieruService.SetUserEnable(id, body.Enable); err != nil {
		jsonMsg(c, "set user enable", err)
		return
	}
	jsonMsg(c, "updated", nil)
}

func (a *MieruController) exportClientJSON(c *gin.Context) {
	inboundID, err := strconv.Atoi(c.Param("inboundId"))
	if err != nil {
		jsonMsg(c, "parse inboundId", err)
		return
	}
	userID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		jsonMsg(c, "parse userId", err)
		return
	}
	serverAddr := c.Query("server")
	if serverAddr == "" {
		serverAddr = c.Request.Host
	}
	data, err := a.mieruService.ExportClientJSON(inboundID, userID, serverAddr)
	if err != nil {
		jsonMsg(c, "export mieru client", err)
		return
	}
	c.Data(200, "application/json", data)
}

func (a *MieruController) exportClientText(c *gin.Context) {
	inboundID, err := strconv.Atoi(c.Param("inboundId"))
	if err != nil {
		jsonMsg(c, "parse inboundId", err)
		return
	}
	userID, err := strconv.Atoi(c.Param("userId"))
	if err != nil {
		jsonMsg(c, "parse userId", err)
		return
	}
	serverAddr := c.Query("server")
	if serverAddr == "" {
		serverAddr = c.Request.Host
	}
	text, err := a.mieruService.ExportClientText(inboundID, userID, serverAddr)
	if err != nil {
		jsonMsg(c, "export mieru client text", err)
		return
	}
	c.String(200, text)
}

// ── Mita lifecycle handlers ───────────────────────────────────────────────────

func (a *MieruController) status(c *gin.Context) {
	s := a.mieruService.GetStatus()
	jsonObj(c, s, nil)
}

func (a *MieruController) start(c *gin.Context) {
	if err := mieru.Start(); err != nil {
		jsonMsg(c, "start mita", err)
		return
	}
	jsonMsg(c, "started", nil)
}

func (a *MieruController) stop(c *gin.Context) {
	if err := mieru.Stop(); err != nil {
		jsonMsg(c, "stop mita", err)
		return
	}
	jsonMsg(c, "stopped", nil)
}
