package i18n

func init() {
	register(LocaleVI, map[string]string{
		// Common validation
		MsgRequired:         "%s là bắt buộc",
		MsgInvalidID:        "ID %s không hợp lệ",
		MsgNotFound:         "không tìm thấy %s: %s",
		MsgAlreadyExists:    "%s đã tồn tại: %s",
		MsgInvalidRequest:   "yêu cầu không hợp lệ: %s",
		MsgInvalidJSON:      "JSON không hợp lệ",
		MsgUnauthorized:     "chưa xác thực",
		MsgPermissionDenied: "từ chối quyền truy cập: %s",
		MsgInternalError:    "lỗi nội bộ: %s",
		MsgInvalidSlug:      "%s phải là slug hợp lệ (chữ thường, số, dấu gạch ngang)",
		MsgFailedToList:     "không thể liệt kê %s",
		MsgFailedToCreate:   "không thể tạo %s: %s",
		MsgFailedToUpdate:   "không thể cập nhật %s: %s",
		MsgFailedToDelete:   "không thể xóa %s: %s",
		MsgFailedToSave:     "không thể lưu %s: %s",
		MsgInvalidUpdates:   "cập nhật không hợp lệ",

		// Agent
		MsgAgentNotFound:       "không tìm thấy agent: %s",
		MsgCannotDeleteDefault: "không thể xóa agent mặc định",
		MsgUserCtxRequired:     "yêu cầu ngữ cảnh người dùng",

		// Chat
		MsgRateLimitExceeded: "vượt quá giới hạn tốc độ — vui lòng đợi",
		MsgNoUserMessage:     "không tìm thấy tin nhắn người dùng",
		MsgUserIDRequired:    "user_id là bắt buộc",
		MsgMsgRequired:       "tin nhắn là bắt buộc",

		// Abort
		MsgAbortStopped:         "đã dừng tác vụ",
		MsgAbortForced:          "buộc dừng tác vụ (vượt quá thời gian chờ 3s)",
		MsgAbortAlreadyAborting: "đang dừng tác vụ",
		MsgAbortNotFound:        "không tìm thấy tác vụ hoặc đã kết thúc",
		MsgAbortUnauthorized:    "không có quyền dừng tác vụ này",
		MsgAbortFailed:          "không thể dừng tác vụ: %s",

		// Channel instances
		MsgInvalidChannelType: "loại channel không hợp lệ",
		MsgInstanceNotFound:   "không tìm thấy phiên bản",

		// Cron
		MsgJobNotFound:                "không tìm thấy tác vụ",
		MsgInvalidCronExpr:            "biểu thức cron không hợp lệ: %s",
		MsgCronDeliverChannelRequired: "cron job có deliver=true cần deliverChannel (tên kênh-instance, ví dụ 'zalo-annhien')",
		MsgCronDeliverToRequired:      "cron job có deliver=true cần deliverTo (chat ID)",

		// Config
		MsgConfigHashMismatch: "cấu hình đã thay đổi (hash không khớp)",

		// Exec approval
		MsgExecApprovalDisabled: "phê duyệt thực thi chưa được bật",

		// Pairing
		MsgSenderChannelRequired: "senderId và channel là bắt buộc",
		MsgCodeRequired:          "mã là bắt buộc",
		MsgSenderIDRequired:      "sender_id là bắt buộc",

		// HTTP API
		MsgInvalidAuth:           "xác thực không hợp lệ",
		MsgMsgsRequired:          "messages là bắt buộc",
		MsgUserIDHeader:          "header X-GoClaw-User-Id là bắt buộc",
		MsgFileTooLarge:          "tệp quá lớn hoặc form multipart không hợp lệ",
		MsgMissingFileField:      "thiếu trường 'file'",
		MsgInvalidFilename:       "tên tệp không hợp lệ",
		MsgChannelKeyReq:         "channel và key là bắt buộc",
		MsgMethodNotAllowed:      "phương thức không được phép",
		MsgStreamingNotSupported: "streaming không được hỗ trợ",
		MsgOwnerOnly:             "chỉ chủ sở hữu mới có thể %s",
		MsgNoAccess:              "không có quyền truy cập %s này",
		MsgAlreadySummoning:      "agent đang được triệu hồi",
		MsgSummoningUnavailable:  "triệu hồi không khả dụng",
		MsgNoDescription:         "agent không có mô tả để triệu hồi lại",
		MsgSummonCancelled:       "đã huỷ triệu hồi",
		MsgCannotCancel:          "agent không trong trạng thái đang triệu hồi",
		MsgInvalidPath:           "đường dẫn không hợp lệ",

		// Tenant backup / restore
		MsgRestoreNewModeRejectsTenantID: "mode=new tạo tenant mới; dùng tenant_slug (không phải tenant_id) làm slug cho tenant mới",

		// Scheduler
		MsgQueueFull:    "hàng đợi session đã đầy",
		MsgShuttingDown: "cổng đang tắt, vui lòng thử lại sau",

		// Provider
		MsgProviderReqFailed: "%s: yêu cầu thất bại: %s",

		// Unknown method
		MsgUnknownMethod: "phương thức không xác định: %s",

		// Not implemented
		MsgNotImplemented: "%s chưa được triển khai",

		// Agent links
		MsgLinksNotConfigured: "liên kết agent chưa được cấu hình",
		MsgInvalidDirection:   "hướng phải là outbound, inbound hoặc bidirectional",
		MsgSourceTargetSame:   "nguồn và đích phải là các agent khác nhau",
		MsgCannotDelegateOpen: "không thể ủy quyền cho agent mở — chỉ agent định sẵn mới có thể là đích ủy quyền",
		MsgNoUpdatesProvided:  "không có cập nhật nào được cung cấp",
		MsgInvalidLinkStatus:  "trạng thái phải là active hoặc disabled",

		// Teams
		MsgTeamsNotConfigured:   "nhóm chưa được cấu hình",
		MsgAgentIsTeamLead:      "agent đã là trưởng nhóm",
		MsgCannotRemoveTeamLead: "không thể xóa trưởng nhóm",

		// Channels
		MsgCannotDeleteDefaultInst: "không thể xóa phiên bản channel mặc định",
		MsgCannotRemoveLastWriter:  "không thể xóa người quản lý cuối cùng",

		// Skills
		MsgSkillsUpdateNotSupported: "skills.update không được hỗ trợ với skill dựa trên tệp",
		MsgCannotResolveSkillID:     "không thể xác định ID skill dựa trên tệp",
		MsgSkillManagedOverwrite:    "Skill này do gcplane quản lý. Cập nhật qua gcplane apply, hoặc tải lên lại với force_imperative=true (sẽ ghi audit log).",
		MsgSkillInvalidSource:       "giá trị source không hợp lệ %q; chỉ chấp nhận: unknown, cli, gcplane",
		MsgInvalidVisibility:        "visibility không hợp lệ %q: phải là private hoặc public",

		// Logs
		MsgInvalidLogAction: "action phải là 'start' hoặc 'stop'",

		// Config
		MsgRawConfigRequired:     "cấu hình raw là bắt buộc",
		MsgRawPatchRequired:      "patch raw là bắt buộc",
		MsgConfigMasterScopeOnly: "config.* chỉ áp dụng cho master scope; dùng endpoint tenant tool config cho override theo tenant",
		MsgMasterScopeRequired:   "thao tác này yêu cầu phạm vi tenant chính",

		// Storage / File
		MsgCannotDeleteSkillsDir: "không thể xóa thư mục skill",
		MsgFailedToReadFile:      "không thể đọc tệp",
		MsgFileNotFound:          "không tìm thấy tệp",
		MsgInvalidVersion:        "phiên bản không hợp lệ",
		MsgVersionNotFound:       "không tìm thấy phiên bản",
		MsgFailedToDeleteFile:    "không thể xóa",

		// OAuth
		MsgNoPendingOAuth:    "không có luồng OAuth đang chờ",
		MsgFailedToSaveToken: "không thể lưu token",

		// Intent Classify
		MsgStatusWorking:       "🔄 Mình đang xử lý yêu cầu của bạn... Vui lòng chờ.",
		MsgStatusDetailed:      "🔄 Mình đang xử lý yêu cầu của bạn...\n%s (lần lặp %d)\nĐã chạy: %s\n\nVui lòng chờ — mình sẽ phản hồi khi xong.",
		MsgStatusPhaseThinking: "Giai đoạn: Đang suy nghĩ...",
		MsgStatusPhaseToolExec: "Giai đoạn: Đang chạy %s",
		MsgStatusPhaseTools:    "Giai đoạn: Đang thực thi công cụ...",
		MsgStatusPhaseCompact:  "Giai đoạn: Đang nén ngữ cảnh...",
		MsgStatusPhaseDefault:  "Giai đoạn: Đang xử lý...",
		MsgCancelledReply:      "✋ Đã hủy. Bạn muốn làm gì tiếp?",
		MsgInjectedAck:         "Đã nhận, tôi sẽ xử lý trong tác vụ hiện tại.",

		// Knowledge Graph
		MsgEntityIDRequired:       "entity_id là bắt buộc",
		MsgEntityFieldsRequired:   "external_id, name và entity_type là bắt buộc",
		MsgTextRequired:           "text là bắt buộc",
		MsgProviderModelRequired:  "provider và model là bắt buộc",
		MsgInvalidProviderOrModel: "provider hoặc model không hợp lệ",

		// Mô tả công cụ tích hợp
		MsgToolReadFile:        "Đọc nội dung tệp từ workspace của agent theo đường dẫn",
		MsgToolWriteFile:       "Ghi nội dung vào tệp trong workspace, tự động tạo thư mục nếu cần",
		MsgToolListFiles:       "Liệt kê tệp và thư mục trong đường dẫn chỉ định",
		MsgToolEdit:            "Chỉnh sửa tệp bằng cách tìm và thay thế đoạn văn bản cụ thể",
		MsgToolExec:            "Thực thi lệnh shell trong workspace và trả về kết quả",
		MsgToolWebSearch:       "Tìm kiếm thông tin trên web bằng công cụ tìm kiếm (Brave hoặc DuckDuckGo)",
		MsgToolWebFetch:        "Tải trang web hoặc API endpoint và trích xuất nội dung văn bản",
		MsgToolMemorySearch:    "Tìm kiếm trong bộ nhớ dài hạn của agent bằng độ tương đồng ngữ nghĩa",
		MsgToolMemoryGet:       "Lấy tài liệu bộ nhớ cụ thể theo đường dẫn tệp",
		MsgToolKGSearch:        "Tìm kiếm thực thể, quan hệ và ghi chú trong đồ thị tri thức của agent",
		MsgToolReadImage:       "Phân tích hình ảnh bằng nhà cung cấp LLM có khả năng nhìn",
		MsgToolReadDocument:    "Phân tích tài liệu (PDF, Word, Excel, PowerPoint, CSV, v.v.) bằng LLM",
		MsgToolCreateImage:     "Tạo hình ảnh từ mô tả văn bản bằng nhà cung cấp tạo ảnh AI",
		MsgToolReadAudio:       "Phân tích tệp âm thanh (giọng nói, nhạc, âm thanh) bằng LLM",
		MsgToolReadVideo:       "Phân tích tệp video bằng nhà cung cấp LLM có khả năng xử lý video",
		MsgToolCreateVideo:     "Tạo video từ mô tả văn bản bằng AI",
		MsgToolCreateAudio:     "Tạo nhạc hoặc hiệu ứng âm thanh từ mô tả văn bản bằng AI",
		MsgToolTTS:             "Chuyển văn bản thành giọng nói tự nhiên",
		MsgToolBrowser:         "Tự động hóa trình duyệt: điều hướng trang, click, điền form, chụp ảnh màn hình",
		MsgToolSessionsList:    "Liệt kê các phiên chat đang hoạt động trên tất cả kênh",
		MsgToolSessionStatus:   "Xem trạng thái và thông tin chi tiết của một phiên chat",
		MsgToolSessionsHistory: "Xem lịch sử tin nhắn của một phiên chat cụ thể",
		MsgToolSessionsSend:    "Gửi tin nhắn vào một phiên chat đang hoạt động thay mặt agent",
		MsgToolMessage:         "Gửi tin nhắn chủ động đến người dùng trên kênh đã kết nối (Telegram, Discord, v.v.)",
		MsgToolCron:            "Lên lịch hoặc quản lý tác vụ định kỳ bằng biểu thức cron, giờ cố định, hoặc khoảng thời gian",
		MsgToolSpawn:           "Tạo subagent chạy nền hoặc giao việc cho agent đã liên kết",
		MsgToolSkillSearch:     "Tìm kiếm kỹ năng có sẵn theo từ khóa hoặc mô tả",
		MsgToolUseSkill:        "Kích hoạt kỹ năng để sử dụng khả năng chuyên biệt (đánh dấu tracing)",
		MsgToolSkillManage:     "Tạo, sửa hoặc xóa kỹ năng từ trải nghiệm hội thoại",
		MsgToolPublishSkill:    "Đăng ký thư mục kỹ năng vào hệ thống, cho phép tìm kiếm và cấp quyền",
		MsgToolTeamTasks:       "Xem, tạo, cập nhật và hoàn thành tác vụ trên bảng tác vụ nhóm",

		MsgSkillNudgePostscript: "Tác vụ này cần nhiều bước. Bạn muốn tôi lưu quy trình này thành kỹ năng tái sử dụng không? Trả lời **\"lưu kỹ năng\"** hoặc **\"bỏ qua\"**.",
		MsgSkillNudge70Pct:      "[System] Bạn đã dùng 70% ngân sách vòng lặp. Cân nhắc xem các mẫu trong phiên này có nên lưu thành kỹ năng không.",
		MsgSkillNudge90Pct:      "[System] Bạn đã dùng 90% ngân sách vòng lặp. Nếu phiên này có quy trình tái sử dụng, hãy cân nhắc lưu thành kỹ năng trước khi hoàn thành.",

		MsgInvalidRole: "vai trò không hợp lệ: giá trị cho phép là owner, admin, operator, member, viewer",

		MsgContactIDsRequired:  "contact_ids là bắt buộc",
		MsgMergeTargetRequired: "cần chính xác một trong tenant_user_id hoặc create_user",
		MsgTenantUserNotFound:  "không tìm thấy tenant user",
		MsgTenantMismatch:      "tenant user không thuộc tenant này",
		MsgTenantScopeRequired: "cần xác định tenant để thực hiện thao tác này",

		// TTS / Giọng đọc
		MsgTtsUnknownModel:       "model tts không hỗ trợ: %s",
		MsgVoicesListFailed:      "không tải được danh sách giọng đọc: %s",
		MsgTtsGeminiInvalidVoice: "giọng đọc Gemini không hợp lệ: %s",
		MsgTtsGeminiSpeakerLimit: "Gemini TTS hỗ trợ tối đa 2 người nói",
		MsgTtsGeminiInvalidModel:  "mô hình Gemini TTS không hợp lệ: %s",
		MsgTtsGeminiTextOnly:      "Gemini từ chối tạo âm thanh. Vui lòng thử văn bản đơn giản hơn, không dịch hay bình luận.",
		MsgTtsParamOutOfRange:     "tham số TTS %q có giá trị %v nằm ngoài phạm vi [%v, %v]",
		MsgTtsParamUnknownKey:     "tham số TTS %q không được nhà cung cấp này hỗ trợ",
		MsgTtsMiniMaxVoicesFailed: "không tải được danh sách giọng đọc MiniMax: %s",

		// VieNeu
		MsgTtsVieneuSynthesisFailed:   "tổng hợp giọng nói VieNeu thất bại: %s",
		MsgTtsVieneuVoicesFailed:      "không tải được danh sách giọng đọc VieNeu: %s",
		MsgTtsVieneuRefAudioInvalid:   "âm thanh tham chiếu không hợp lệ: %s",
		MsgTtsVieneuDaemonUnreachable: "không kết nối được tới VieNeu; vui lòng dùng image goclaw với ENABLE_FULL_SKILLS",
		MsgVieneuRefAudioTooShort:     "âm thanh tham chiếu quá ngắn: %s",
		MsgVieneuRefAudioTooLong:      "âm thanh tham chiếu quá dài: %s",
		MsgVieneuRefTextRequired:      "cần có ref_text khi nhân bản giọng",
		MsgVieneuMaxClonedVoices:      "đã đạt giới hạn giọng nhân bản cho mỗi tenant (%d)",
		MsgVieneuClonedVoiceNotFound:  "không tìm thấy giọng nhân bản: %s",

		// STT
		MsgSTTAllProvidersFailed:     "Tất cả nhà cung cấp STT đều thất bại",
		MsgSTTLegacyConfigDeprecated: "Cấu hình STT cũ đã lỗi thời; hãy chuyển sang builtin_tools[stt]",
		MsgSTTWhatsappPrivacyWarning: "Bật STT cho WhatsApp sẽ phá vỡ mã hóa đầu cuối cho tin nhắn thoại gửi đến agent này.",
		MsgVoiceMessageFallback:      "[Tin nhắn thoại]",

		// Webhooks
		MsgWebhookAuthFailed:              "xác thực webhook thất bại",
		MsgWebhookHMACInvalid:             "chữ ký HMAC không hợp lệ",
		MsgWebhookHMACTimestampSkew:       "thời gian yêu cầu nằm ngoài cửa sổ chấp nhận",
		MsgWebhookBearerRequiredHMAC:      "webhook này yêu cầu xác thực HMAC",
		MsgWebhookRevoked:                 "webhook đã bị thu hồi",
		MsgWebhookKindMismatch:            "loại yêu cầu không khớp cấu hình webhook",
		MsgWebhookRateLimited:             "vượt quá giới hạn tốc độ webhook",
		MsgWebhookBodyTooLarge:            "nội dung yêu cầu vượt quá giới hạn kích thước",
		MsgWebhookIdempotencyConflict:     "xung đột idempotency key: nội dung yêu cầu không khớp",
		MsgWebhookTenantMismatch:          "tenant của webhook không khớp",
		MsgWebhookAgentNotFound:           "không tìm thấy agent webhook",
		MsgWebhookChannelNotFound:         "không tìm thấy kênh webhook",
		MsgWebhookMediaSSRFBlocked:        "URL media bị chặn bởi chính sách SSRF",
		MsgWebhookMediaTooLarge:           "tệp media vượt quá giới hạn kích thước",
		MsgWebhookMediaMIMEDenied:         "loại MIME của media không được phép",
		MsgWebhookCallbackURLInvalid:      "URL callback không hợp lệ hoặc bị chặn",
		MsgWebhookLLMTimeout:              "LLM xử lý hết thời gian chờ",
		MsgWebhookLaneSaturated:           "làn xử lý webhook đã đầy",
		MsgWebhookLocalhostOnlyViolation:  "webhook này chỉ cho phép gọi từ localhost",
		MsgWebhookMediaChannelUnsupported: "kênh không hỗ trợ tệp đính kèm media",
		MsgWebhookIPDenied:                "địa chỉ IP không nằm trong danh sách cho phép",
		MsgWebhookEncryptionUnavailable:   "khóa mã hóa webhook chưa được cấu hình; hãy đặt GOCLAW_ENCRYPTION_KEY để kích hoạt webhook",

		// Hooks
		// Workstation
		MsgWorkstationNotFound:     "không tìm thấy máy trạm: %s",
		MsgWorkstationKeyExists:    "khóa máy trạm đã được sử dụng: %s",
		MsgInvalidBackend:          "loại backend không hợp lệ: %s (phải là ssh|docker)",
		MsgWorkstationInactive:     "máy trạm không hoạt động: %s",
		MsgInvalidMetadataShape:    "metadata không hợp lệ cho backend %s: %s",
		MsgWorkstationRequired:     "agent chưa được gắn máy trạm; hãy truyền workstation_id",
		MsgWorkstationAccessDenied: "agent %s không được phép truy cập máy trạm %s",
		MsgBackendNotReady:         "backend máy trạm chưa sẵn sàng: %s",

		MsgHookInvalidMatcher:          "biểu thức regex matcher không hợp lệ: %s",
		MsgHookCommandDisabledStandard: "hook loại command chỉ khả dụng trên phiên bản Lite",
		MsgHookPromptRequiresMatcher:   "hook prompt bắt buộc có matcher hoặc if_expr (chống chi phí vượt kiểm soát)",
		MsgHookCircuitBreakerTripped:   "hook đã tự tắt sau nhiều lần thất bại liên tiếp",
		MsgHookBudgetExceeded:          "tenant đã vượt ngân sách token cho hook",
		MsgHookPerTurnCapReached:       "đã đạt giới hạn số lần gọi hook trong một lượt",
		MsgHookBuiltinReadOnly:         "hook dựng sẵn chỉ cho phép bật/tắt, không thể chỉnh sửa",

		// Zalo OA OAuth channel
		MsgZaloOACodeExchangeFailed: "đổi mã xác thực Zalo OAuth thất bại: %s",
		MsgZaloOAInvalidChannelType: "kênh không phải loại zalo_oa",
		MsgZaloOAConnected:           "đã kết nối tài khoản Zalo OA: %s",
		MsgZaloOAInvalidState:        "mã state OAuth không hợp lệ hoặc đã hết hạn",
		MsgZaloOARedirectURIRequired: "credentials.redirect_uri là bắt buộc và phải khớp chính xác với callback đã đăng ký trong Zalo developer console",
		MsgZaloOAMissingAppID:        "credentials.app_id là bắt buộc — hãy nhập app_id cho kênh trước khi yêu cầu URL cấp quyền",
		MsgZaloOAStateGenFailed:      "không thể sinh mã state cấp quyền; vui lòng thử lại",
		MsgZaloOAOAIDMismatch:        "URL callback thuộc về một OA khác — hãy dán URL lấy từ trang cấp quyền của instance NÀY",

		// RPC URL webhook Zalo
		MsgZaloWebhookWrongChannelType: "channels.instances.zalo.webhook_url chỉ áp dụng cho instance zalo_bot hoặc zalo_oa",
		MsgZaloWebhookPathHint:         "Thêm URL công khai của gateway (ví dụ https://gw.example.com) vào trước đường dẫn, rồi đăng ký URL đầy đủ trong Zalo developer console.",

		// Catalog lỗi runtime của Zalo OA. Tham số: (mã int, thông điệp gốc)
		MsgZaloOAErrAuth:              "Zalo từ chối access token sau khi đã làm mới (mã %d: %s); cần ủy quyền lại OA",
		MsgZaloOAErrRefreshExpired:    "Refresh token Zalo đã hết hạn (mã %d: %s); người vận hành phải cấp lại quyền trong OA console",
		MsgZaloOAErrPayload:           "Zalo từ chối nội dung yêu cầu (mã %d: %s); kiểm tra cấu trúc tin nhắn và các trường bắt buộc",
		MsgZaloOAErrSize:              "Tệp tải lên Zalo vượt giới hạn (mã %d: %s); ảnh 1MB / tệp 5MB / gif 5MB",
		MsgZaloOAErrPermission:        "Zalo yêu cầu quyền bổ sung cho thao tác này (mã %d: %s); cấp quyền còn thiếu cho ứng dụng OA",
		MsgZaloOAErrInteractionWindow: "Người nhận đang ngoài cửa sổ tương tác của Zalo (mã %d: %s); chờ người dùng nhắn trước hoặc dùng tin mẫu trả phí",
		MsgZaloOAErrUserNotVisible:    "OA không thấy được người dùng đích (mã %d: %s)",
		MsgZaloOAErrAppDisabled:       "Ứng dụng Zalo đã bị vô hiệu hoặc bị cấm (mã %d: %s); liên hệ hỗ trợ Zalo",
		MsgZaloOAErrRate:              "Quota Zalo đã hết (mã %d: %s); chờ cửa sổ quota làm mới",
		MsgZaloOAErrServer:            "Zalo trả về lỗi server tạm thời (mã %d: %s); thử lại sau",
		MsgZaloOAErrRedirectURI:       "Zalo từ chối OAuth redirect_uri (mã %d: %s); cập nhật redirect URI trong Zalo console khớp với cấu hình kênh",
		MsgZaloOAReauthDueSoon:        "Refresh token sẽ hết hạn trong %d ngày; vui lòng cấp quyền lại OA để tránh gián đoạn",
		MsgZaloOAUnsupportedAttachment: "(Tệp %q (%s) không thể gửi qua Zalo OA — chỉ chấp nhận PDF/DOC/DOCX. Nội dung đã mô tả ở trên.)",

		// Workstation permissions (Phase 6)
		MsgWorkstationCmdDenied:    "lệnh bị từ chối bởi chính sách workstation: %s",
		MsgWorkstationEnvDenied:    "biến môi trường bị từ chối bởi chính sách: %s",
		MsgWorkstationInputInvalid: "lệnh chứa ký tự không hợp lệ: %s",
		MsgWorkstationRateLimit:    "đã vượt quá giới hạn tốc độ workstation",
		MsgWorkstationPermNotFound: "không tìm thấy mục quyền: %s",
		// Workstation activity (Phase 7)
		MsgWorkstationActivityTitle: "Hoạt động gần đây",
		MsgWorkstationActionExec:    "Thực thi",
		MsgWorkstationActionDeny:    "Từ chối",

		// Package updates (Phase 4+5)
		MsgPackageNotInstalled:  "Gói %s chưa được cài đặt",
		MsgPackageUpdateLocked:  "Gói %s đang được cập nhật bởi một yêu cầu khác",
		MsgReleaseNotFound:      "Không tìm thấy phiên bản %s cho %s",
		MsgAssetNotFound:        "Không có tệp tương thích cho %s/%s",
		MsgChecksumMismatch:     "Checksum không khớp cho %s",
		MsgUpdateSwapFailed:     "Không cài được %s; đã khôi phục phiên bản cũ",
		MsgUpdateManifestDesync: "Binary đã cập nhật nhưng lưu manifest thất bại — cần khôi phục thủ công cho %s",
		MsgUpdateCacheStale:     "Cache cập nhật đã cũ; hãy refresh trước khi áp dụng",

		// Grant env validation
		MsgGrantEnvDeniedKeys:   "các khóa env không được phép: %s",
		MsgGrantEnvValueInvalid: "giá trị env không hợp lệ: %s",
		MsgGrantEnvTooManyKeys:  "quá nhiều khóa env: tối đa 50",
		MsgGrantEnvRevealLimit:  "đã vượt giới hạn yêu cầu xem env — vui lòng thử lại sau",

		// Secure CLI execution
		MsgSecureCliBinaryNotFound: "binary %q chưa được đăng ký cho secure exec",
		MsgSecureCliNoGrant:        "agent chưa được cấp quyền cho binary %q",
		MsgSecureCliDeniedByPolicy: "lời gọi bị từ chối bởi chính sách deny_args: %s",

		// OAuth integrations
		MsgOAuthStateMismatch:       "token trạng thái OAuth không khớp hoặc đã hết hạn — vui lòng thử lại",
		MsgOAuthExchangeFailed:      "trao đổi mã OAuth thất bại: %s",
		MsgOAuthBinaryNotFound:      "binary %q chưa được đăng ký cho tenant này",
		MsgOAuthIntegrationNotFound: "không tìm thấy tích hợp nào cho %q",
		MsgOAuthRevoked:             "thông tin đăng nhập Google đã bị thu hồi — vui lòng kết nối lại qua Settings → Integrations",
		MsgOAuthNotConfigured:       "Google OAuth chưa được cấu hình trên máy chủ này",

		// Standby mode
		StandbyToolDescription:      "Tạm dừng trả lời trong cuộc trò chuyện hiện tại. Trợ lý vẫn quan sát và ghi nhớ tin nhắn nhưng sẽ không trả lời cho đến khi hết thời gian tạm dừng.",
		StandbyToolParamDuration:    "Thời gian tạm dừng tính bằng giây (60-86400).",
		StandbyToolParamReason:      "Lý do (tùy chọn) được ghi lại cùng lần tạm dừng.",
		StandbyErrorInvalidDuration: "duration_seconds phải nằm trong khoảng 60 đến 86400",
		StandbyErrorNoChannelCtx:    "enter_standby cần ngữ cảnh channel và không gọi được từ caller này",
		StandbyEntered:              "Đã vào chế độ chờ trong %s (lý do: %s)",
		StandbyRPCInvalidSchedule:   "lịch không hợp lệ: %s",
		StandbyRPCNoPermission:      "cần quyền admin của tenant để sửa lịch channel",

		TeamCaptureRPCNoPermission:    "cần quyền admin để bật/tắt thu thập trả lời của team",
		TeamCaptureRPCInvalidConfig:   "cấu hình capture không hợp lệ: %s",
		TeamCaptureJudgeAgentNotFound: "không tìm thấy judge agent %q trong tenant — hãy tạo agent này hoặc chọn agent_key khác",
		TeamCaptureJudgeKeyRequired:   "phải có judge_agent_key khi bật judge_evaluation",
		TeamCaptureScheduleInvalid:    "lịch judge không hợp lệ %q — dùng biểu thức cron 5 trường",
		TeamEvalNotFound:              "không tìm thấy đánh giá trả lời của team",
		TeamEvalJudgeError:            "đánh giá thất bại: %s",

		TraceRetryPayloadOversize: "Tải dữ liệu quá lớn để chạy lại (>2 MB).",
		TraceRetryLocked:          "Đang chạy lại — vui lòng chờ.",
		TraceRetryAgentGone:       "Agent của trace này đã bị xóa.",
		TraceRetryProviderGone:    "Provider của trace này đã bị xóa.",
		TraceRetryPayloadMissing:  "Dữ liệu replay không còn khả dụng.",
		TraceRetryConfirmRequired: "Lần chạy này đã gửi tin nhắn — xác nhận để chạy lại.",
		TraceRetryStarted:         "Đã bắt đầu chạy lại.",
		TraceRetryNotFailed:       "Chỉ có thể chạy lại trace đã kết thúc (run vẫn đang chạy).",

		// Message tool cross-target forward notice
		MessageCrossTargetForwarded: "📤 Đã forward sang %s theo yêu cầu: %q",

		// Package update source labels
		MsgPackagesUpdatesSourceGithub: "GitHub",
		MsgPackagesUpdatesSourcePip:    "pip",
		MsgPackagesUpdatesSourceNpm:    "npm",
		MsgPackagesUpdatesSourceApk:    "apk",

		// Package update availability messages
		MsgPackagesUpdatesUnavailablePip: "pip chưa cài trên hệ thống",
		MsgPackagesUpdatesUnavailableNpm: "npm chưa cài trên hệ thống",
		MsgPackagesUpdatesUnavailableApk: "apk không khả dụng trên hệ thống này",

		// Package update failure reasons
		MsgPackagesUpdatesReasonDependencyConflict: "Xung đột phụ thuộc",
		MsgPackagesUpdatesReasonPermission:         "Bị từ chối quyền",
		MsgPackagesUpdatesReasonNetwork:            "Lỗi mạng",
		MsgPackagesUpdatesReasonNotFound:           "Không tìm thấy gói",
		MsgPackagesUpdatesReasonTargetMissing:      "Phiên bản không tồn tại",
		MsgPackagesUpdatesReasonExternallyManaged:  "Môi trường được quản lý bên ngoài",
		MsgPackagesUpdatesReasonLocked:             "Cơ sở dữ liệu gói đang bị khóa",
		MsgPackagesUpdatesReasonDiskFull:           "Đĩa đã đầy",
		MsgPackagesUpdatesReasonHelperUnavailable:  "Dịch vụ đặc quyền không khả dụng",
	})
}
