-- name: ListAllOrders :many
-- @api GET /orders
-- @page page.OrderList
-- @scoped customer_id
SELECT id, customer_id, total, status
FROM orders
WHERE status = @status
ORDER BY id;
