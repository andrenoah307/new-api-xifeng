package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.RouteTag("api"))
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	anonymousRequestBodyLimit := middleware.AnonymousRequestBodyLimit()
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", anonymousRequestBodyLimit, controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/cn-disclaimer", controller.GetCnDisclaimer)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricing)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("pricing"))
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/rankings", middleware.HeaderNavModuleAuth("rankings"), controller.GetRankings)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.POST("/oauth/email/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", anonymousRequestBodyLimit, controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", anonymousRequestBodyLimit, controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", anonymousRequestBodyLimit, controller.WaffoWebhook)
		// :env separates test vs prod URLs so the operator can register each
		// in Pancake's matching webhook slot; handler enforces env match.
		apiRouter.POST("/waffo-pancake/webhook/:env", anonymousRequestBodyLimit, controller.WaffoPancakeWebhook)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.POST("/epay/notify", anonymousRequestBodyLimit, controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.POST("/risk_warning/ack", controller.AcknowledgeRiskWarning)
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", middleware.CriticalRateLimit(), controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/invitation_codes", controller.GetMyInvitationCodes)
				selfRoute.GET("/invitation_codes/quota", controller.GetMyInvitationCodeQuota)
				selfRoute.POST("/invitation_codes", controller.GenerateMyInvitationCode)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				selfRoute.POST("/discount_code/validate", controller.ValidateUserDiscountCode)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.GET("/commission_records/self", controller.GetMyCommissionRecords)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/commission_records", controller.GetAllCommissionRecords)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin)
				adminRoute.DELETE("/:id/bindings/:binding_type", controller.AdminClearUserBinding)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}

		// Subscription billing (plans, purchase, admin management)
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionRoute.POST("/balance/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestBalancePay)
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
			subscriptionRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestWaffoPancakePay)
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)
			subscriptionAdminRoute.POST("/plans/:id/subscriptions/reset", controller.AdminResetPlanSubscriptions)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/users/:id/subscriptions/reset", controller.AdminResetUserSubscriptionsByPlan)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", anonymousRequestBodyLimit, controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", anonymousRequestBodyLimit, controller.SubscriptionEpayReturn)

		ticketRoute := apiRouter.Group("/ticket")
		ticketRoute.Use(middleware.UserAuth())
		{
			ticketRoute.GET("/limit-status", controller.GetTicketLimitStatus)
			ticketRoute.POST("/", controller.CreateTicket)
			ticketRoute.GET("/self", controller.GetUserTickets)
			ticketRoute.GET("/self/:id", controller.GetUserTicket)
			ticketRoute.POST("/self/:id/message", controller.CreateUserTicketMessage)
			ticketRoute.PUT("/self/:id/close", controller.CloseUserTicket)
			ticketRoute.GET("/invoice/eligible_orders", controller.GetEligibleInvoiceOrders)
			ticketRoute.POST("/invoice/", controller.CreateInvoiceTicket)
			ticketRoute.GET("/refund/invoice-check", controller.CheckRefundInvoiceConflict)
			ticketRoute.GET("/invoice/refund-check", controller.CheckInvoiceRefundConflict)
			ticketRoute.POST("/refund/", controller.CreateRefundTicket)
			ticketRoute.POST("/attachment", middleware.CriticalRateLimit(), controller.UploadTicketAttachment)
			ticketRoute.DELETE("/attachment/:id", controller.DeleteTicketAttachment)
		}

		// 附件预览/下载独立路由：浏览器直接发起的 <img src> / <a href> 无法注入自定义
		// 请求头，因此使用 SessionAuth（cookie-only）而非 UserAuth。
		// 仅幂等的 GET 资源放在这里。
		ticketAttachmentDownloadRoute := apiRouter.Group("/ticket/attachment")
		ticketAttachmentDownloadRoute.Use(middleware.SessionAuth())
		{
			ticketAttachmentDownloadRoute.GET("/:id", controller.DownloadTicketAttachment)
		}

		// 工单后台路由：客服（role=5）可以访问，客服仅能处理分配给自己或所在组未分配的工单，
		// 这层可见性在 controller 里通过 ensureTicketAccessible 二次校验。
		ticketAdminRoute := apiRouter.Group("/ticket/admin")
		ticketAdminRoute.Use(middleware.TicketStaffAuth())
		{
			ticketAdminRoute.GET("/", controller.GetAllTickets)
			ticketAdminRoute.GET("/staff", controller.ListTicketStaff)
			ticketAdminRoute.GET("/invoice/export-list", controller.GetInvoiceExportList)
			ticketAdminRoute.GET("/:id", controller.GetTicket)
			ticketAdminRoute.GET("/:id/user-profile", controller.GetTicketUserProfile)
			ticketAdminRoute.GET("/:id/user-topups", controller.GetTicketUserTopUps)
			ticketAdminRoute.POST("/:id/message", controller.CreateAdminTicketMessage)
			ticketAdminRoute.PUT("/:id/status", controller.UpdateTicketStatus)
			ticketAdminRoute.PUT("/:id/assign", controller.AssignTicket)
			ticketAdminRoute.GET("/:id/invoice", controller.GetTicketInvoice)
			ticketAdminRoute.PUT("/:id/invoice/status", controller.UpdateInvoiceStatus)
			ticketAdminRoute.GET("/:id/refund", controller.GetTicketRefund)
			ticketAdminRoute.PUT("/:id/refund/status", controller.UpdateRefundStatus)
		}

		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.POST("/payment_compliance", controller.ConfirmPaymentCompliance)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
			optionRoute.GET("/waffo-pancake/catalog", controller.ListWaffoPancakeCatalog)
			optionRoute.POST("/waffo-pancake/pair", controller.CreateWaffoPancakePair)
			optionRoute.POST("/waffo-pancake/save", controller.SaveWaffoPancake)
			optionRoute.POST("/waffo-pancake/subscription-product", controller.CreateWaffoPancakeSubscriptionProduct)
			optionRoute.GET("/waffo-pancake/subscription-product-options", controller.ListWaffoPancakeSubscriptionProductOptions)
			optionRoute.GET("/email_templates", controller.ListEmailTemplates)
			optionRoute.POST("/email_templates/preview", controller.PreviewEmailTemplate)
			optionRoute.POST("/email_templates/reset", controller.ResetEmailTemplate)
			optionRoute.POST("/email_templates/test", controller.SendEmailTemplateTest)
		}

		// Custom OAuth provider management (root only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", controller.FetchCustomOAuthDiscovery)
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
			performanceRoute.GET("/logs", controller.GetLogFiles)
			performanceRoute.DELETE("/logs", controller.CleanupLogFiles)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		riskRoute := apiRouter.Group("/risk")
		riskRoute.Use(middleware.AdminAuth())
		{
			riskRoute.GET("/overview", controller.GetRiskCenterOverview)
			riskRoute.GET("/config", controller.GetRiskCenterConfig)
			riskRoute.GET("/detect-ip", controller.DetectRiskIP)
			riskRoute.PUT("/config", controller.UpdateRiskCenterConfig)
			riskRoute.GET("/groups", controller.GetRiskGroups)
			riskRoute.GET("/rules", controller.GetRiskRules)
			riskRoute.POST("/rules", controller.CreateRiskRule)
			riskRoute.PUT("/rules/:id", controller.UpdateRiskRule)
			riskRoute.DELETE("/rules/:id", controller.DeleteRiskRule)
			riskRoute.GET("/subjects", controller.GetRiskSubjects)
			riskRoute.POST("/subjects/:scope/:id/unblock", controller.UnblockRiskSubject)
			riskRoute.GET("/incidents", controller.GetRiskIncidents)

			// omni-moderation tab
			riskRoute.GET("/moderation/config", controller.GetModerationConfig)
			riskRoute.PUT("/moderation/config", controller.UpdateModerationConfig)
			riskRoute.GET("/moderation/overview", controller.GetModerationOverview)
			riskRoute.GET("/moderation/incidents", controller.GetModerationIncidents)
			riskRoute.GET("/moderation/incidents/:id", controller.GetModerationIncidentDetail)
			riskRoute.POST("/moderation/debug", controller.SubmitModerationDebug)
			riskRoute.GET("/moderation/debug/:id", controller.GetModerationDebugResult)
			riskRoute.GET("/moderation/categories", controller.GetModerationCategories)
			riskRoute.GET("/moderation/rules", controller.GetModerationRules)
			riskRoute.POST("/moderation/rules", controller.CreateModerationRule)
			riskRoute.PUT("/moderation/rules/:id", controller.UpdateModerationRule)
			riskRoute.DELETE("/moderation/rules/:id", controller.DeleteModerationRule)
			riskRoute.GET("/moderation/queue_stats", controller.GetModerationQueueStats)

			// enforcement layer (unified post-hit handling)
			riskRoute.GET("/enforcement/config", controller.GetEnforcementConfig)
			riskRoute.PUT("/enforcement/config", controller.UpdateEnforcementConfig)
			riskRoute.GET("/enforcement/overview", controller.GetEnforcementOverview)
			riskRoute.GET("/enforcement/incidents", controller.GetEnforcementIncidents)
			riskRoute.GET("/enforcement/counters", controller.GetEnforcementCounters)
			riskRoute.POST("/enforcement/users/:id/reset_counter", controller.ResetEnforcementCounter)
			riskRoute.POST("/enforcement/users/:id/unban", controller.UnbanEnforcementUser)
			riskRoute.POST("/enforcement/test_email", controller.SendEnforcementTestEmail)
		}
		autoGroupRoute := apiRouter.Group("/auto_group")
		autoGroupRoute.Use(middleware.AdminAuth())
		{
			autoGroupRoute.GET("/rules", controller.GetAutoGroupRules)
			autoGroupRoute.GET("/rules/:id", controller.GetAutoGroupRule)
			autoGroupRoute.POST("/rules", controller.CreateAutoGroupRule)
			autoGroupRoute.PUT("/rules/:id", controller.UpdateAutoGroupRule)
			autoGroupRoute.DELETE("/rules/:id", controller.DeleteAutoGroupRule)
			autoGroupRoute.GET("/enrollments", controller.GetAutoGroupEnrollments)
			autoGroupRoute.POST("/enrollments", controller.CreateAutoGroupEnrollment)
			autoGroupRoute.DELETE("/enrollments/:id", controller.DeleteAutoGroupEnrollment)
			autoGroupRoute.POST("/sweep", controller.TriggerAutoGroupSweep)
		}
		monitoringAdminRoute := apiRouter.Group("/monitoring/admin")
		monitoringAdminRoute.Use(middleware.AdminAuth())
		{
			monitoringAdminRoute.GET("/groups", controller.GetAdminMonitoringGroups)
			monitoringAdminRoute.GET("/groups/:group", controller.GetAdminMonitoringGroupDetail)
			monitoringAdminRoute.GET("/groups/:group/history", controller.GetAdminMonitoringGroupHistory)
			monitoringAdminRoute.POST("/refresh", middleware.CriticalRateLimit(), controller.RefreshMonitoringData)
			monitoringAdminRoute.DELETE("/groups/:group/records", controller.DeleteMonitoringGroupRecords)
		}
		monitoringPublicRoute := apiRouter.Group("/monitoring/public")
		monitoringPublicRoute.Use(middleware.TryUserAuth())
		{
			monitoringPublicRoute.GET("/groups", controller.GetPublicMonitoringGroups)
			monitoringPublicRoute.GET("/groups/:group/history", controller.GetPublicMonitoringGroupHistory)
		}
		registerChannelRoutes(apiRouter)
		registerAuthzRoutes(apiRouter)
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		invitationCodeRoute := apiRouter.Group("/invitation_code")
		invitationCodeRoute.Use(middleware.AdminAuth())
		{
			invitationCodeRoute.GET("/", controller.GetAllInvitationCodes)
			invitationCodeRoute.GET("/search", controller.SearchInvitationCodes)
			invitationCodeRoute.GET("/:id", controller.GetInvitationCode)
			invitationCodeRoute.GET("/:id/usages", controller.GetInvitationCodeUsages)
			invitationCodeRoute.POST("/", controller.AddInvitationCode)
			invitationCodeRoute.PUT("/", controller.UpdateInvitationCode)
			invitationCodeRoute.DELETE("/invalid", controller.DeleteInvalidInvitationCodes)
			invitationCodeRoute.DELETE("/:id", controller.DeleteInvitationCode)
		}
		discountCodeRoute := apiRouter.Group("/discount_code")
		discountCodeRoute.Use(middleware.AdminAuth())
		{
			discountCodeRoute.GET("/", controller.GetAllDiscountCodes)
			discountCodeRoute.GET("/search", controller.SearchDiscountCodes)
			discountCodeRoute.GET("/:id", controller.GetDiscountCode)
			discountCodeRoute.POST("/", controller.AddDiscountCode)
			discountCodeRoute.PUT("/", controller.UpdateDiscountCode)
			discountCodeRoute.DELETE("/:id", controller.DeleteDiscountCode)
			discountCodeRoute.POST("/:id/cleanup", controller.CleanupDiscountCodePendingOrders)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), middleware.LogQueryRateLimit(), controller.GetAllLogs)
		// Legacy synchronous direct-delete route used only by the classic frontend.
		// TODO: remove once the classic frontend is removed; the default frontend uses /system-task/log-cleanup.
		logRoute.DELETE("/", middleware.RootAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), middleware.LogQueryRateLimit(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), middleware.LogQueryRateLimit(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), middleware.LogQueryRateLimit(), controller.GetUserLogs)
		logRoute.GET("/export", middleware.AdminAuth(), controller.ExportAllLogsCsv)
		logRoute.GET("/self/export", middleware.UserAuth(), middleware.LogExportRateLimit(), controller.ExportUserLogsCsv)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)
		// Offline export - user
		logRoute.POST("/self/export-offline", middleware.UserAuth(), middleware.LogOfflineExportRateLimit(), controller.SubmitOfflineExport)
		logRoute.GET("/self/export-tasks", middleware.UserAuth(), controller.GetUserExportTaskList)
		logRoute.GET("/self/export-download/:id", middleware.SessionAuth(), middleware.LogExportDownloadRateLimit(), controller.DownloadExportFile)
		// Offline export - admin
		logRoute.GET("/export-tasks", middleware.AdminAuth(), controller.GetAdminExportTasks)
		logRoute.GET("/export-queue-stats", middleware.AdminAuth(), controller.GetExportQueueStats)
		logRoute.GET("/export-config", middleware.AdminAuth(), controller.GetExportConfig)
		logRoute.PUT("/export-config", middleware.AdminAuth(), controller.UpdateExportConfig)
		logRoute.POST("/export-tasks/:id/cancel", middleware.AdminAuth(), controller.CancelExportTask)
		logRoute.GET("/export-tasks/:id/download", middleware.AdminAuth(), controller.AdminDownloadExportFile)

		systemTaskRoute := apiRouter.Group("/system-task")
		systemTaskRoute.Use(middleware.RootAuth())
		{
			systemTaskRoute.POST("/log-cleanup", controller.CreateLogCleanupSystemTask)
			systemTaskRoute.GET("/list", controller.ListSystemTasks)
			systemTaskRoute.GET("/current", controller.GetCurrentSystemTask)
			systemTaskRoute.GET("/:task_id", controller.GetSystemTask)
		}
		systemInfoRoute := apiRouter.Group("/system-info")
		systemInfoRoute.Use(middleware.RootAuth())
		{
			systemInfoRoute.GET("/instances", controller.ListSystemInstances)
			systemInfoRoute.DELETE("/stale-instances", controller.DeleteStaleSystemInstances)
			systemInfoRoute.DELETE("/instances/:node_name", controller.DeleteStaleSystemInstance)
		}

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)
		dataRoute.GET("/self", middleware.UserAuth(), middleware.DashboardDataRateLimit(), controller.GetUserQuotaDates)
		dataRoute.GET("/flow", middleware.AdminAuth(), controller.GetAllFlowQuotaDates)
		dataRoute.GET("/flow/self", middleware.UserAuth(), controller.GetUserFlowQuotaDates)

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)
		}
	}
}
