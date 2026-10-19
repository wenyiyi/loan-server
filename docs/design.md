# 设计说明

模块化单体：Gin → Service → Repository → PostgreSQL。领域状态只能按 `PROPOSED → APPROVED → INVESTED → DISBURSED` 转换。审批和放款保存材料、员工和业务日期，审计表保存状态变化。

投资事务首先 `SELECT ... FOR UPDATE` 锁贷款，再检查持久化幂等记录，校验状态和余额，写投资明细、更新汇总及状态。满额时在同一事务生成审计和按投资者去重的 Outbox 行。任何步骤失败全部回滚，外部邮件发送不在该事务内。

同贷款的并发幂等请求由贷款行锁串行化。跨贷款相同 key 的竞争由数据库唯一约束裁决；失败事务先回滚，再查询已提交记录，返回幂等冲突。首次响应 JSON 持久化，因此重试不依赖当前贷款状态。

数据库约束兜底本金额度、状态枚举、利率和满额条件；投资合计这一跨表不变量由统一写入事务维护。应避免其他系统直接写表。审批/放款的主键确保每贷款只有一份记录。

Outbox 通过 `FOR UPDATE SKIP LOCKED` 原子领取，租约过期可重新领取。随机租约 token 避免旧执行者确认新租约。失败后持久化退避，成功后记录 delivered_at。SMTP 无跨系统原子提交能力，因此采用至少一次投递，不能宣称严格一次。

数据访问采用 GORM：统一 model 映射现有 PostgreSQL 表并直接用于响应，使用 WithContext 传播取消与超时，Transaction 管理提交/回滚，clause.Locking 保留贷款行锁。批量 Outbox 入队及租约领取保留原子 SQL，并统一通过 GORM 执行。不引入 Redis 或消息队列。静态 Bearer 认证实现便于本地验证，生产需接入身份服务。协议生成/对象存储、实际资金渠道、偿还、币种转换、自动扩缩容与端到端邮件送达确认不在当前范围。

API 与通知消费分别由 `cmd/server` 和 `cmd/worker` 启动。`config.NewConfig` 通过 Viper 加载 YAML 和环境变量，`repository.NewDB` 管理数据库连接，`internal/server` 组装路由，`internal/middleware` 提供认证、角色授权、请求日志和链路关联。每张表有独立 Repository：Loan、Approval、Disbursement、Investment、LoanStatusHistory 和 NotificationOutbox，各自只访问对应表。Service 编排跨表业务，并把同一个事务连接传给各 Repository。

风格参考 marvel-main：使用 New 构造函数，入口显式注入依赖，handler 使用 api/v1 的统一响应，zap 日志通过请求 context 关联 trace_id。贷款和投资 ID 在 Go 与 JSON 边界统一为字符串，PostgreSQL 保持 BIGSERIAL；金额保持 int64。

数据库结构继续由 SQL migrations 管理，不在启动时调用 AutoMigrate。GORM 的 SQL 日志关闭以避免输出业务参数。rate/roi 使用 numeric(10,6)，自增主键使用 bigint；应用时间字段使用 Unix 毫秒整数。Compose 只挂载 001_init.up.sql；001_init.down.sql 仅用于显式手动回滚，不在正常启动时执行。

模型集中在 internal/model，不再保留 domain 层及模型转换。Loan 的 remaining_amount 通过 GORM hooks 计算，标记 gorm:"-" 不参与持久化；投资明细直接返回 LoanInvestment，敏感字段由 json:"-" 排除。

集成测试通过 integration 构建标签启用，通过 DATABASE_URL 与应用共用数据库及账号。测试先校验连接实际数据库，再在随机 schema 中执行全部 SQL 迁移，通过真实 HTTP 路由覆盖完整链路。清理由关闭连接后 DROP SCHEMA 完成，不依赖外层事务覆盖 Service 的业务事务。
