import "@typespec/http";
import "@typespec/openapi";

using Http;
using OpenAPI;

/** Orders service: read orders by id. */
@service(#{ title: "Orders API" })
@info(#{ version: "0.1.0" })
namespace Orders;

/** Lifecycle of an order. */
enum OrderStatus {
  created,
  paid,
  cancelled,
  done,
}

/**
 * Money as an exact decimal string, never a float.
 * Example: { "amount": "1999.90", "currency": "EUR" }.
 */
model Money {
  /** Decimal number with up to 2 digits after the point. */
  @pattern("^-?\\d+(\\.\\d{1,2})?$")
  @example("1999.90")
  amount: string;

  /** ISO 4217 currency code. */
  @pattern("^[A-Z]{3}$")
  @example("EUR")
  currency: string;
}

/** A customer order. */
model Order {
  /** Order id. */
  @minValue(1)
  id: int64;

  /** User who owns the order. */
  @minValue(1)
  user_id: int64;

  created_date: utcDateTime;
  last_changed_date: utcDateTime;

  /** User who made the last change. */
  @minValue(1)
  last_changed_by: int64;

  total_amount: Money;

  @minLength(1)
  delivery_address: string;

  status: OrderStatus;
}

/** Error body for every error response. */
model ErrorBody {
  /** Machine-readable code, for example "order_not_found". */
  code: string;

  /** Text for humans. */
  message: string;
}

@error
model NotFoundError {
  @statusCode _: 404;
  @body body: ErrorBody;
}

@error
model BadRequestError {
  @statusCode _: 400;
  @body body: ErrorBody;
}

@route("/orders")
@tag("Orders")
interface OrdersApi {
  /** Get one order by its id. */
  @get
  @operationId("getOrderById")
  getById(@path @minValue(1) id: int64): Order | BadRequestError | NotFoundError;
}
