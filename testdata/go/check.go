package sample

func dummyFunction(s string) string {
	return "unused " + s
}

/**
 * @api {POST} /user/info some user info
 * @apiVersion 0.1.0
 * @apiGroup webrpc
 *
 * @apiParam {number} user_id 用户 ID
 *
 * @apiSuccess (200) {number} code CodeSuccess = 0.
 * @apiSuccess (200) {object} info
 * @apiSuccess (200) {string} info.id 用户 ID
 * @apiSuccess (200) {number} info.username 用户名
 * @apiSuccess (200) {number} info.userintro 用户简介
 * @apiSuccess (200) {number} info.confirm 用户权限或身份标识
 * @apiSuccess (200) {number} info.iconurl icon url
 * @apiSuccess (200) {number} info.authenticated 加 V 认证标识，从 confirm 中得来的
 * @apiSuccess (200) {number} info.fansnum 粉丝数
 * @apiSuccess (200) {number} info.follownum 关注数
 * @apiSuccess (200) {number} info.ban 是否在黑名单
 *
 * @apiError (500) {number}
 * @apiError (500) {string} info 相关错误信息
 */
