-- name: ListCustomerOrders :many
-- @api GET /orders
-- @page page.OrderList
-- @scoped customer_id
SELECT id, total, status
FROM orders
WHERE customer_id = @customer_id AND deleted_at IS NULL
ORDER BY id;

-- name: CreateOrder :one
-- @api POST /orders
-- @requires orders:write
-- @scoped customer_id
-- @fragment page.OrderCard
INSERT INTO orders (customer_id, total, status)
VALUES (@customer_id, @total, @status)
RETURNING id, total, status;
