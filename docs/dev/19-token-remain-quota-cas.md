# Token remain_quota 编辑 Lost Update —— 待办：全栈 CAS（S2）

> 状态：**暂缓 / 待排期**（2026-06-18 由会话评审确认）
> 关联坑点：[`99-pitfalls.md#128`](99-pitfalls.md)（Lost Update：整行/整 hash 覆盖原子计数列）
> 来源：`tmp/newapi-lost-update-poc`（漏洞 #7）评审外溢出的同类问题

---

## 1. 背景

排查 PoC（高并发 `PUT /api/user/self` 钉死 `users.quota` 实现免费铸币）时，发现 **Token 编辑路径**存在**同类但成因不同**的 Lost Update：编辑 token（改名/改额度等）会把客户端传入的**旧 `remain_quota` 绝对值**写回 DB，与并发扣费的原子自减竞争。

本会话已落地的 S1/S3/新①②③ 均为**纯后端、收口式**修复（详见 #128）。**Token 的 DB 侧绝对值覆盖（本文 S2）单独暂缓**，因为它需要前端配合，属全栈改动。

> 重要边界：token 消费**同时扣 `user.quota`**。S1 修好后，`remain_quota` 被覆盖**不再产生"免费额度"**——它只是 token 子额度/统计层面的不一致。故 S2 是正确性加固，**非新的免费铸币口子**，可安全延后。

---

## 2. 分析链条

### 2.1 漏洞路径
```
PUT /api/token/  (UpdateToken, controller/token.go:250)
  → cleanToken, _ := model.GetTokenByIds(token.Id, userId)   // 从 DB 读当前整行（含 remain_quota）
  → cleanToken.RemainQuota = token.RemainQuota                // ← 用客户端传入的绝对值覆盖（controller/token.go:295）
  → cleanToken.Update()                                       // model/token.go:298
      → DB.Model(token).Select("name","status","expired_time",
            "remain_quota", ...).Updates(token)               // ← Select 白名单显式含 remain_quota，整列写回绝对值
```
并发扣费（正确、原子）：
```
decreaseTokenQuota (model/token.go:425)
  → DB.Model(&Token{}).Where("id=?",id).Updates({
        "remain_quota": gorm.Expr("remain_quota - ?", quota), ...})
```
两者交错 → 编辑写的旧绝对值覆盖原子自减结果 = Lost Update。

### 2.2 为什么"加锁/重读"救不了
绝对值覆盖一个**被并发自减的余额**，本质上无法靠 `FOR UPDATE` 行锁解决：

- **FOR UPDATE 绝对 set**：锁内 set `remain=X`；并发扣费阻塞→提交后 `X-c`。但护盾反复 set 同一 `X` 时，每次都把扣掉的 `c` 退回 → 余额钉死在 `X`（退款）。锁只防撕裂写，防不住"绝对覆盖丢消费"的**语义**。
- **delta vs 新鲜 DB 值**：`delta = clientX − DB当前值`，原子 `remain += delta`。护盾持续发固定旧 `X` 时，随扣费降低，`delta` 变正 → **把消费退回**，等价钉死。同样失败。

### 2.3 唯一正确解：compare-and-swap（需客户端基线）
必须用**客户端编辑时所基于的基线值**做 CAS，而非服务端任意时刻的当前值：
```
UPDATE tokens SET remain_quota = :new
 WHERE id = :id AND remain_quota = :baseline      -- baseline = 前端加载编辑表单时的 remain_quota
```
- 命中 1 行 → 期间无并发改动，编辑生效；
- 命中 0 行 → 期间被扣费/他处改动 → 返回冲突，前端提示"额度已变动，请刷新后重试"。

护盾失效原因：一旦扣费改动 `remain_quota`，攻击者手里的旧 `baseline` 不再等于 DB 当前值，CAS 必 0 行命中，绝对覆盖打不进去。

> 注：本会话已修复 Token **缓存侧**整 hash 覆盖（`GetTokenById`/`Token.Update`/`SelectUpdate` 的 `cacheSetToken`→`cacheDeleteToken`，见 #128 新③）。**剩余的就是本文的 DB 侧绝对值覆盖**，与缓存修复正交。

---

## 3. 结论

- **方案**：全栈 CAS（前端回传基线 + 后端条件更新 + 冲突提示）。
- **范围**：后端 1 处 + 前端两套（Classic / Default）。
- **优先级**：中（修好 S1 后无免费额度风险，属一致性加固）。
- **风险**：极少数并发编辑会收到冲突需重试（可接受，正确 UX）。

---

## 4. 待办清单（TODO）

### 4.1 后端
- [ ] `dto`/请求结构：`UpdateToken` 增加可选基线字段 `original_remain_quota *int`（指针，区分"未传/显式 0"，遵循 DTO 红线 #Rule 7）。
- [ ] `controller/token.go:UpdateToken`：非 `status_only` 且 `remain_quota` 变化时，走 CAS 分支；不再无条件 `cleanToken.RemainQuota = token.RemainQuota`。
- [ ] `model/token.go`：新增 `UpdateTokenRemainQuotaCAS(id, baseline, newVal)`（`Where("id=? AND remain_quota=?")`，返回 `RowsAffected`），或在 `UpdateToken` 内组合"非额度字段走 `Token.Update`（移除 `remain_quota`）+ 额度走 CAS"。
- [ ] CAS 0 行命中 → 返回明确错误码（如 `token_remain_quota_conflict`），不复用通用 `ApiError`，便于前端区分。
- [ ] `unlimited_quota` 切换、`remain_quota` 与 `unlimited` 同改的边界：无限额时跳过 CAS。
- [ ] 三库兼容核对（`Where` 条件三库通用，无需特殊处理）。

### 4.2 前端 Classic
- [ ] `web/classic/src/components/table/tokens/modals/EditTokenModal.jsx`：表单加载时记录 `original_remain_quota`，提交时回传；处理冲突响应 → Toast「额度已变动，请刷新后重试」+ 自动重拉。

### 4.3 前端 Default
- [ ] `web/default/src/features/keys/*`（`api-key-form.ts` / `api-keys-mutate-drawer.tsx`）：同上，提交 `original_remain_quota`，处理冲突分支。

### 4.4 测试
- [ ] `model` 层：CAS 命中/未命中用例（baseline 匹配→写入；baseline 失配→0 行、余额不被覆盖）。
- [ ] 并发场景（确定性模拟）：扣费改动 `remain_quota` 后，旧 baseline 的编辑必失败、不覆盖。

### 4.5 文档
- [ ] 实施后更新本文档状态为"已完成"，并在 `99-pitfalls.md#128` 把 S2 从"暂缓"移到"已修"。

---

## 5. 参考
- 漏洞原理与已修项：`docs/dev/99-pitfalls.md` #128
- PoC：`tmp/newapi-lost-update-poc/`（README / poc.py / shield.py）
- 关键代码：`controller/token.go:250,295`、`model/token.go:298,425`、`model/token_cache.go`
