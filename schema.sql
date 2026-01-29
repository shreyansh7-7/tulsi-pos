-- Users & Roles
CREATE TABLE IF NOT EXISTS roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_by INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id INT REFERENCES users(id),
    role_id INT REFERENCES roles(id),
    PRIMARY KEY (user_id, role_id)
);

-- Suppliers
CREATE TABLE IF NOT EXISTS suppliers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    contact_info VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Products
CREATE TABLE IF NOT EXISTS products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    sku VARCHAR(100) UNIQUE,
    barcode VARCHAR(100),
    hsn_code VARCHAR(50),
    gender VARCHAR(20),
    category VARCHAR(50),
    purchase_price NUMERIC(10, 2),
    sales_price NUMERIC(10, 2),
    gst_percent NUMERIC(5, 2),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Purchase Invoices
CREATE TABLE IF NOT EXISTS purchase_invoices (
    id SERIAL PRIMARY KEY,
    invoice_number VARCHAR(100) NOT NULL,
    supplier_id INT REFERENCES suppliers(id),
    total_amount_before_discount NUMERIC(12, 2),
    discount_amount NUMERIC(12, 2),
    total_gst NUMERIC(12, 2),
    total_invoice_amount NUMERIC(12, 2),
    total_items INT,
    total_quantity INT,
    notes TEXT,
    created_by INT REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS purchase_invoice_items (
    id SERIAL PRIMARY KEY,
    purchase_invoice_id INT REFERENCES purchase_invoices(id),
    product_id INT REFERENCES products(id),
    quantity INT,
    purchase_price NUMERIC(10, 2),
    gst_percent NUMERIC(5, 2),
    gst_amount NUMERIC(10, 2),
    line_total NUMERIC(12, 2)
);

-- View for ListPurchases
CREATE OR REPLACE VIEW purchases AS
SELECT pi.id, pi.invoice_number, s.name as supplier_name, pi.created_at, pi.deleted_at
FROM purchase_invoices pi
LEFT JOIN suppliers s ON pi.supplier_id = s.id;

-- Sales Invoices
CREATE TABLE IF NOT EXISTS sales_invoices (
    id SERIAL PRIMARY KEY,
    invoice_number VARCHAR(100) UNIQUE NOT NULL,
    customer_name VARCHAR(100),
    customer_mobile VARCHAR(20),
    status VARCHAR(20) DEFAULT 'DRAFT', -- DRAFT, INVOICED
    total_amount_before_discount NUMERIC(12, 2),
    discount_type VARCHAR(20), -- INR, PERCENT
    discount_value NUMERIC(10, 2),
    total_discount NUMERIC(12, 2),
    taxable_amount NUMERIC(12, 2),
    total_gst NUMERIC(12, 2),
    round_off NUMERIC(5, 2),
    total_invoice_amount NUMERIC(12, 2),
    total_items INT,
    total_quantity INT,
    payment_mode VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP,
    invoice_pdf_key TEXT
);

CREATE TABLE IF NOT EXISTS sales_invoice_items (
    id SERIAL PRIMARY KEY,
    sales_invoice_id INT REFERENCES sales_invoices(id),
    product_id INT REFERENCES products(id),
    quantity INT,
    mrp NUMERIC(10, 2),
    sales_rate NUMERIC(10, 2),
    discount_type VARCHAR(20),
    discount_value NUMERIC(10, 2),
    discount_amount NUMERIC(10, 2),
    gst_percent NUMERIC(5, 2),
    gst_amount NUMERIC(10, 2),
    line_total NUMERIC(12, 2),
    deleted_at TIMESTAMP
);

-- Inventory
CREATE TABLE IF NOT EXISTS inventory_transactions (
    id SERIAL PRIMARY KEY,
    product_id INT REFERENCES products(id),
    quantity INT, -- can be negative
    ref_type VARCHAR(50), -- purchase, sale
    ref_id INT,
    created_by INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Seed initial data
INSERT INTO roles (name) VALUES ('admin'), ('cashier') ON CONFLICT DO NOTHING;
