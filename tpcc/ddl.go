package tpcc

import (
	"context"
	"fmt"
	"strings"
)

const (
	tableItem      = "item"
	tableCustomer  = "customer"
	tableDistrict  = "district"
	tableOrders    = "orders"
	tableNewOrder  = "new_order"
	tableOrderLine = "order_line"
	tableHistory   = "history"
	tableWareHouse = "warehouse"
	tableStock     = "stock"
)

type ddlManager struct {
	parts             int
	warehouses        int
	partitionType     int
	useFK             bool
	useClusteredIndex bool
}

func newDDLManager(parts int, useFK bool, warehouses, partitionType int, useClusteredIndex bool) *ddlManager {
	return &ddlManager{parts: parts, useFK: useFK, warehouses: warehouses, partitionType: partitionType, useClusteredIndex: useClusteredIndex}
}

func (w *ddlManager) createTableDDL(ctx context.Context, query string, tableName string) error {
	s := getTPCCState(ctx)
	fmt.Printf("creating table %s\n", tableName)
	if _, err := s.Conn.ExecContext(ctx, query); err != nil {
		return err
	}
	return nil
}

func (w *ddlManager) createIndexDDL(ctx context.Context, query string, indexName string) error {
	s := getTPCCState(ctx)
	fmt.Printf("creating index %s\n", indexName)
	if _, err := s.Conn.ExecContext(ctx, query); err != nil {
		return err
	}
	return nil
}

func (w *ddlManager) createForeignKeyDDL(ctx context.Context, query string, indexName string) error {
	s := getTPCCState(ctx)
	fmt.Printf("creating foreign key %s\n", indexName)
	if _, err := s.Conn.ExecContext(ctx, query); err != nil {
		return err
	}
	return nil
}

func (w *ddlManager) createCitusDDL(ctx context.Context, query string, action string) error {
	s := getTPCCState(ctx)
	fmt.Println(action)
	if _, err := s.Conn.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("%s failed: %w", action, err)
	}
	return nil
}

func (w *ddlManager) appendPartition(query string, partKeys string) string {
	if w.parts <= 1 {
		return query
	}
	if w.partitionType == PartitionTypeListAsHash {
		// Generate LIST partitions equivalent with HASH partitions
		s := fmt.Sprintf("%s\nPARTITION BY LIST (%s)\n(", query, partKeys)
		for i := 0; i < w.parts; i++ {
			if i > 0 {
				s = s + ",\n "
			}
			var part string
			for j := i; j < w.warehouses; j = j + w.parts {
				if j > i {
					part = part + ","
				}
				part = part + fmt.Sprintf("%d", j+1)
			}
			s = fmt.Sprintf("%sPARTITION p%d VALUES IN (%s)", s, i, part)
		}
		return s + ")"
	} else if w.partitionType == PartitionTypeListAsRange {
		// Generate LIST partitions equivalent with RANGE partitions
		s := fmt.Sprintf("%s\nPARTITION BY LIST (%s)\n(", query, partKeys)
		addedWarehouses := 0
		for i := 0; i < w.parts; i++ {
			if i > 0 {
				s = s + ",\n "
			}
			warehousesToAdd := w.warehouses - addedWarehouses
			partsLeft := w.parts - i
			warehousesPerPartition := warehousesToAdd / partsLeft
			if (warehousesToAdd % partsLeft) != 0 {
				warehousesPerPartition++
			}
			var part string
			for j := 0; j < warehousesPerPartition; j++ {
				if j > 0 {
					part = part + ","
				}
				addedWarehouses++
				part = part + fmt.Sprintf("%d", addedWarehouses)
			}
			s = fmt.Sprintf("%sPARTITION p%d VALUES IN (%s)", s, i, part)
		}
		return s + ")"
	} else if w.partitionType == PartitionTypeRange {
		// Generate RANGE partitions
		s := fmt.Sprintf("%s\nPARTITION BY RANGE (%s)\n(", query, partKeys)
		for i := 0; i < w.parts; i++ {
			if i > 0 {
				s = s + ",\n "
			}
			warehousesPerPartition := w.warehouses / w.parts
			if (w.warehouses % w.parts) != 0 {
				warehousesPerPartition++
			}
			s = fmt.Sprintf("%sPARTITION p%d VALUES LESS THAN (%d)", s, i, 1+(i+1)*warehousesPerPartition)
		}
		return s + ")"
	}

	return fmt.Sprintf("%s\nPARTITION BY HASH(%s)\nPARTITIONS %d", query, partKeys, w.parts)
}

// createTables creates tables schema.
func (w *ddlManager) createTables(ctx context.Context, driver string) error {
	if driver == "mysql" {

		var clusteredIndexType string
		if w.useClusteredIndex {
			clusteredIndexType = "CLUSTERED"
		} else {
			clusteredIndexType = "NONCLUSTERED"
		}

		// Warehouse
		query := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS warehouse (
	w_id INT NOT NULL,
	w_name VARCHAR(10),
	w_street_1 VARCHAR(20),
	w_street_2 VARCHAR(20),
	w_city VARCHAR(20),
	w_state CHAR(2),
	w_zip CHAR(9),
	w_tax DECIMAL(4, 4),
	w_ytd DECIMAL(12, 2),
	PRIMARY KEY (w_id) /*T![clustered_index] %s */
)`, clusteredIndexType)

		query = w.appendPartition(query, "w_id")

		if err := w.createTableDDL(ctx, query, tableWareHouse); err != nil {
			return err
		}

		// District
		query = fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS district (
	d_id INT NOT NULL,
	d_w_id INT NOT NULL,
	d_name VARCHAR(10),
	d_street_1 VARCHAR(20),
	d_street_2 VARCHAR(20),
	d_city VARCHAR(20),
	d_state CHAR(2),
	d_zip CHAR(9),
	d_tax DECIMAL(4, 4),
	d_ytd DECIMAL(12, 2),
	d_next_o_id INT,
	PRIMARY KEY (d_w_id, d_id) /*T![clustered_index] %s */
)`, clusteredIndexType)

		query = w.appendPartition(query, "d_w_id")

		if err := w.createTableDDL(ctx, query, tableDistrict); err != nil {
			return err
		}

		// Customer
		query = fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS customer (
	c_id INT NOT NULL,
	c_d_id INT NOT NULL,
	c_w_id INT NOT NULL,
	c_first VARCHAR(16),
	c_middle CHAR(2),
	c_last VARCHAR(16),
	c_street_1 VARCHAR(20),
	c_street_2 VARCHAR(20),
	c_city VARCHAR(20),
	c_state CHAR(2),
	c_zip CHAR(9),
	c_phone CHAR(16),
	c_since DATETIME,
	c_credit CHAR(2),
	c_credit_lim DECIMAL(12, 2),
	c_discount DECIMAL(4,4),
	c_balance DECIMAL(12,2),
	c_ytd_payment DECIMAL(12,2),
	c_payment_cnt INT,
	c_delivery_cnt INT,
	c_data VARCHAR(500),
	PRIMARY KEY(c_w_id, c_d_id, c_id) /*T![clustered_index] %s */,
	INDEX idx_customer (c_w_id, c_d_id, c_last, c_first)
)`, clusteredIndexType)

		query = w.appendPartition(query, "c_w_id")

		if err := w.createTableDDL(ctx, query, tableCustomer); err != nil {
			return err
		}

		query = `
CREATE TABLE IF NOT EXISTS history (
	h_c_id INT NOT NULL,
	h_c_d_id INT NOT NULL,
	h_c_w_id INT NOT NULL,
	h_d_id INT NOT NULL,
	h_w_id INT NOT NULL,
	h_date DATETIME,
	h_amount DECIMAL(6, 2),
	h_data VARCHAR(24),
	INDEX idx_h_w_id (h_w_id),
	INDEX idx_h_c_w_id (h_c_w_id)
)`

		query = w.appendPartition(query, "h_w_id")

		if err := w.createTableDDL(ctx, query, tableHistory); err != nil {
			return err
		}

		query = fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS new_order (
	no_o_id INT NOT NULL,
	no_d_id INT NOT NULL,
	no_w_id INT NOT NULL,
	PRIMARY KEY(no_w_id, no_d_id, no_o_id) /*T![clustered_index] %s */
)`, clusteredIndexType)

		query = w.appendPartition(query, "no_w_id")
		if err := w.createTableDDL(ctx, query, tableNewOrder); err != nil {
			return err
		}

		// because order is a keyword, so here we use orders instead
		query = fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS orders (
	o_id INT NOT NULL,
	o_d_id INT NOT NULL,
	o_w_id INT NOT NULL,
	o_c_id INT,
	o_entry_d DATETIME,
	o_carrier_id INT,
	o_ol_cnt INT,
	o_all_local INT,
	PRIMARY KEY(o_w_id, o_d_id, o_id) /*T![clustered_index] %s */,
	INDEX idx_order (o_w_id, o_d_id, o_c_id, o_id)
)`, clusteredIndexType)

		query = w.appendPartition(query, "o_w_id")
		if err := w.createTableDDL(ctx, query, tableOrders); err != nil {
			return err
		}

		query = fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS order_line (
		ol_o_id INT NOT NULL,
		ol_d_id INT NOT NULL,
		ol_w_id INT NOT NULL,
		ol_number INT NOT NULL,
		ol_i_id INT NOT NULL,
		ol_supply_w_id INT,
		ol_delivery_d DATETIME,
		ol_quantity INT,
		ol_amount DECIMAL(6, 2),
		ol_dist_info CHAR(24),
		PRIMARY KEY(ol_w_id, ol_d_id, ol_o_id, ol_number) /*T![clustered_index] %s */
)`, clusteredIndexType)

		query = w.appendPartition(query, "ol_w_id")
		if err := w.createTableDDL(ctx, query, tableOrderLine); err != nil {
			return err
		}

		query = fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS stock (
	s_i_id INT NOT NULL,
	s_w_id INT NOT NULL,
	s_quantity INT,
	s_dist_01 CHAR(24),
	s_dist_02 CHAR(24),
	s_dist_03 CHAR(24),
	s_dist_04 CHAR(24),
	s_dist_05 CHAR(24),
	s_dist_06 CHAR(24),
	s_dist_07 CHAR(24),
	s_dist_08 CHAR(24),
	s_dist_09 CHAR(24),
	s_dist_10 CHAR(24),
	s_ytd INT,
	s_order_cnt INT,
	s_remote_cnt INT,
	s_data VARCHAR(50),
	PRIMARY KEY(s_w_id, s_i_id) /*T![clustered_index] %s */
)`, clusteredIndexType)

		query = w.appendPartition(query, "s_w_id")
		if err := w.createTableDDL(ctx, query, tableStock); err != nil {
			return err
		}

		query = fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS item (
	i_id INT NOT NULL,
	i_im_id INT,
	i_name VARCHAR(24),
	i_price DECIMAL(5, 2),
	i_data VARCHAR(50),
	PRIMARY KEY(i_id) /*T![clustered_index] %s */
)`, clusteredIndexType)

		if err := w.createTableDDL(ctx, query, tableItem); err != nil {
			return err
		}

		if w.useFK {
			query = `
alter table district add constraint d_warehouse_fkey
    foreign key (d_w_id)
    references warehouse (w_id)`
			if err := w.createIndexDDL(ctx, query, "d_warehouse_fkey"); err != nil {
				return err
			}

			query = `
alter table customer add constraint c_district_fkey
    foreign key (c_w_id, c_d_id)
    references district (d_w_id, d_id)`
			if err := w.createIndexDDL(ctx, query, "c_district_fkey"); err != nil {
				return err
			}

			query = `
alter table history add constraint h_customer_fkey
    foreign key (h_c_w_id, h_c_d_id, h_c_id)
    references customer (c_w_id, c_d_id, c_id)`
			if err := w.createIndexDDL(ctx, query, "h_customer_fkey"); err != nil {
				return err
			}

			query = `
alter table history add constraint h_district_fkey
    foreign key (h_w_id, h_d_id)
    references district (d_w_id, d_id)`
			if err := w.createIndexDDL(ctx, query, "h_district_fkey"); err != nil {
				return err
			}

			query = `
alter table new_order add constraint no_order_fkey
    foreign key (no_w_id, no_d_id, no_o_id)
    references orders (o_w_id, o_d_id, o_id)`
			if err := w.createIndexDDL(ctx, query, "no_order_fkey"); err != nil {
				return err
			}

			query = `
alter table orders add constraint o_customer_fkey
    foreign key (o_w_id, o_d_id, o_c_id)
    references customer (c_w_id, c_d_id, c_id)`
			if err := w.createIndexDDL(ctx, query, "o_customer_fkey"); err != nil {
				return err
			}

			query = `
alter table order_line add constraint ol_order_fkey
    foreign key (ol_w_id, ol_d_id, ol_o_id)
    references orders (o_w_id, o_d_id, o_id)`
			if err := w.createIndexDDL(ctx, query, "ol_order_fkey"); err != nil {
				return err
			}

			query = `
alter table order_line add constraint ol_stock_fkey
    foreign key (ol_supply_w_id, ol_i_id)
    references stock (s_w_id, s_i_id)`
			if err := w.createIndexDDL(ctx, query, "ol_stock_fkey"); err != nil {
				return err
			}

			query = `
alter table stock add constraint s_warehouse_fkey
    foreign key (s_w_id)
    references warehouse (w_id)`
			if err := w.createIndexDDL(ctx, query, "s_warehouse_fkey"); err != nil {
				return err
			}

			query = `
alter table stock add constraint s_item_fkey
    foreign key (s_i_id)
    references item (i_id)`
			if err := w.createIndexDDL(ctx, query, "s_item_fkey"); err != nil {
				return err
			}
		}

		if w.parts > 1 {
			// TODO: add PARTITION

		}

	} else if driver == "postgres" {
		// Warehouse
		query := `
CREATE TABLE IF NOT EXISTS warehouse (
	w_id INT NOT NULL,
	w_name VARCHAR(10),
	w_street_1 VARCHAR(20),
	w_street_2 VARCHAR(20),
	w_city VARCHAR(20),
	w_state CHAR(2),
	w_zip CHAR(9),
	w_tax DECIMAL(4, 4),
	w_ytd DECIMAL(12, 2),
	PRIMARY KEY (w_id)
)`

		if err := w.createTableDDL(ctx, query, tableWareHouse); err != nil {
			return err
		}

		// District
		query = `
CREATE TABLE IF NOT EXISTS district (
	d_id INT NOT NULL,
	d_w_id INT NOT NULL,
	d_name VARCHAR(10),
	d_street_1 VARCHAR(20),
	d_street_2 VARCHAR(20),
	d_city VARCHAR(20),
	d_state CHAR(2),
	d_zip CHAR(9),
	d_tax DECIMAL(4, 4),
	d_ytd DECIMAL(12, 2),
	d_next_o_id INT,
	PRIMARY KEY (d_w_id, d_id)
)`

		if err := w.createTableDDL(ctx, query, tableDistrict); err != nil {
			return err
		}

		// Customer
		query = `
CREATE TABLE IF NOT EXISTS customer (
	c_id INT NOT NULL,
	c_d_id INT NOT NULL,
	c_w_id INT NOT NULL,
	c_first VARCHAR(16),
	c_middle CHAR(2),
	c_last VARCHAR(16),
	c_street_1 VARCHAR(20),
	c_street_2 VARCHAR(20),
	c_city VARCHAR(20),
	c_state CHAR(2),
	c_zip CHAR(9),
	c_phone CHAR(16),
	c_since TIMESTAMP,
	c_credit CHAR(2),
	c_credit_lim DECIMAL(12, 2),
	c_discount DECIMAL(4,4),
	c_balance DECIMAL(12,2),
	c_ytd_payment DECIMAL(12,2),
	c_payment_cnt INT,
	c_delivery_cnt INT,
	c_data VARCHAR(500),
	PRIMARY KEY(c_w_id, c_d_id, c_id)
)`

		if err := w.createTableDDL(ctx, query, tableCustomer); err != nil {
			return err
		}
		if err := w.createIndexDDL(ctx, "create index idx_customer on customer(c_w_id, c_d_id, c_last, c_first)", "idx_customer"); err != nil {
			return err
		}

		query = `
CREATE TABLE IF NOT EXISTS history (
	h_c_id INT NOT NULL,
	h_c_d_id INT NOT NULL,
	h_c_w_id INT NOT NULL,
	h_d_id INT NOT NULL,
	h_w_id INT NOT NULL,
	h_date TIMESTAMP,
	h_amount DECIMAL(6, 2),
	h_data VARCHAR(24)
)`
		if err := w.createTableDDL(ctx, query, tableHistory); err != nil {
			return err
		}

		if err := w.createIndexDDL(ctx, "create index idx_h_w_id on history(h_w_id)", "idx_h_w_id"); err != nil {
			return err
		}
		if err := w.createIndexDDL(ctx, "create index idx_h_c_w_id on history(h_c_w_id)", "idx_h_c_w_id"); err != nil {
			return err
		}

		query = `
CREATE TABLE IF NOT EXISTS new_order (
	no_o_id INT NOT NULL,
	no_d_id INT NOT NULL,
	no_w_id INT NOT NULL,
	PRIMARY KEY(no_w_id, no_d_id, no_o_id)
)`

		if err := w.createTableDDL(ctx, query, tableNewOrder); err != nil {
			return err
		}

		// because order is a keyword, so here we use orders instead
		query = `
CREATE TABLE IF NOT EXISTS orders (
	o_id INT NOT NULL,
	o_d_id INT NOT NULL,
	o_w_id INT NOT NULL,
	o_c_id INT,
	o_entry_d TIMESTAMP,
	o_carrier_id INT,
	o_ol_cnt INT,
	o_all_local INT,
	PRIMARY KEY(o_w_id, o_d_id, o_id)
)`

		if err := w.createTableDDL(ctx, query, tableOrders); err != nil {
			return err
		}

		if err := w.createIndexDDL(ctx, "create index idx_order on orders(o_w_id, o_d_id, o_c_id, o_id)", "idx_order"); err != nil {
			return err
		}

		query = `
	CREATE TABLE IF NOT EXISTS order_line (
		ol_o_id INT NOT NULL,
		ol_d_id INT NOT NULL,
		ol_w_id INT NOT NULL,
		ol_number INT NOT NULL,
		ol_i_id INT NOT NULL,
		ol_supply_w_id INT,
		ol_delivery_d TIMESTAMP,
		ol_quantity INT,
		ol_amount DECIMAL(6, 2),
		ol_dist_info CHAR(24),
		PRIMARY KEY(ol_w_id, ol_d_id, ol_o_id, ol_number)
)`

		if err := w.createTableDDL(ctx, query, tableOrderLine); err != nil {
			return err
		}

		query = `
CREATE TABLE IF NOT EXISTS stock (
	s_i_id INT NOT NULL,
	s_w_id INT NOT NULL,
	s_quantity INT,
	s_dist_01 CHAR(24),
	s_dist_02 CHAR(24),
	s_dist_03 CHAR(24),
	s_dist_04 CHAR(24),
	s_dist_05 CHAR(24),
	s_dist_06 CHAR(24),
	s_dist_07 CHAR(24),
	s_dist_08 CHAR(24),
	s_dist_09 CHAR(24),
	s_dist_10 CHAR(24),
	s_ytd INT,
	s_order_cnt INT,
	s_remote_cnt INT,
	s_data VARCHAR(50),
	PRIMARY KEY(s_w_id, s_i_id)
)`

		if err := w.createTableDDL(ctx, query, tableStock); err != nil {
			return err
		}

		query = `
CREATE TABLE IF NOT EXISTS item (
	i_id INT NOT NULL,
	i_im_id INT,
	i_name VARCHAR(24),
	i_price DECIMAL(5, 2),
	i_data VARCHAR(50),
	PRIMARY KEY(i_id)
)`
		if err := w.createTableDDL(ctx, query, tableItem); err != nil {
			return err
		}

		if w.useFK {
			query = `
alter table district add constraint d_warehouse_fkey
    foreign key (d_w_id)
    references warehouse (w_id)`
			if err := w.createIndexDDL(ctx, query, "d_warehouse_fkey"); err != nil {
				return err
			}

			query = `
alter table customer add constraint c_district_fkey
    foreign key (c_w_id, c_d_id)
    references district (d_w_id, d_id)`
			if err := w.createIndexDDL(ctx, query, "c_district_fkey"); err != nil {
				return err
			}

			query = `
alter table history add constraint h_customer_fkey
    foreign key (h_c_w_id, h_c_d_id, h_c_id)
    references customer (c_w_id, c_d_id, c_id)`
			if err := w.createIndexDDL(ctx, query, "h_customer_fkey"); err != nil {
				return err
			}

			query = `
alter table history add constraint h_district_fkey
    foreign key (h_w_id, h_d_id)
    references district (d_w_id, d_id)`
			if err := w.createIndexDDL(ctx, query, "h_district_fkey"); err != nil {
				return err
			}

			query = `
alter table new_order add constraint no_order_fkey
    foreign key (no_w_id, no_d_id, no_o_id)
    references orders (o_w_id, o_d_id, o_id)`
			if err := w.createIndexDDL(ctx, query, "no_order_fkey"); err != nil {
				return err
			}

			query = `
alter table orders add constraint o_customer_fkey
    foreign key (o_w_id, o_d_id, o_c_id)
    references customer (c_w_id, c_d_id, c_id)`
			if err := w.createIndexDDL(ctx, query, "o_customer_fkey"); err != nil {
				return err
			}

			query = `
alter table order_line add constraint ol_order_fkey
    foreign key (ol_w_id, ol_d_id, ol_o_id)
    references orders (o_w_id, o_d_id, o_id)`
			if err := w.createIndexDDL(ctx, query, "ol_order_fkey"); err != nil {
				return err
			}

			query = `
alter table order_line add constraint ol_stock_fkey
    foreign key (ol_supply_w_id, ol_i_id)
    references stock (s_w_id, s_i_id)`
			if err := w.createIndexDDL(ctx, query, "ol_stock_fkey"); err != nil {
				return err
			}

			query = `
alter table stock add constraint s_warehouse_fkey
    foreign key (s_w_id)
    references warehouse (w_id)`
			if err := w.createIndexDDL(ctx, query, "s_warehouse_fkey"); err != nil {
				return err
			}

			query = `
alter table stock add constraint s_item_fkey
    foreign key (s_i_id)
    references item (i_id)`
			if err := w.createIndexDDL(ctx, query, "s_item_fkey"); err != nil {
				return err
			}
		}

		if w.parts > 1 {
			// TODO: add PARTITION

		}

	}

	return nil
}

func (w *ddlManager) createCitusTables(ctx context.Context) error {
	if err := w.createCitusDDL(ctx, "CREATE EXTENSION IF NOT EXISTS citus", "creating citus extension"); err != nil {
		return err
	}
	if err := w.checkCitusWorkers(ctx); err != nil {
		return err
	}

	referenceTables := []string{tableItem}
	for _, table := range referenceTables {
		query := fmt.Sprintf(`
SELECT create_reference_table('%[1]s')
WHERE NOT EXISTS (
	SELECT 1 FROM pg_dist_partition WHERE logicalrelid = '%[1]s'::regclass
)`, table)
		if err := w.createCitusDDL(ctx, query, fmt.Sprintf("creating citus reference table %s", table)); err != nil {
			return err
		}
	}

	distributedTables := []struct {
		table        string
		column       string
		colocateWith string
	}{
		{table: tableWareHouse, column: "w_id"},
		{table: tableDistrict, column: "d_w_id", colocateWith: tableWareHouse},
		{table: tableCustomer, column: "c_w_id", colocateWith: tableWareHouse},
		{table: tableHistory, column: "h_w_id", colocateWith: tableWareHouse},
		{table: tableNewOrder, column: "no_w_id", colocateWith: tableWareHouse},
		{table: tableOrders, column: "o_w_id", colocateWith: tableWareHouse},
		{table: tableOrderLine, column: "ol_w_id", colocateWith: tableWareHouse},
		{table: tableStock, column: "s_w_id", colocateWith: tableWareHouse},
	}
	for _, dist := range distributedTables {
		createFn := fmt.Sprintf("create_distributed_table('%s', '%s')", dist.table, dist.column)
		if dist.colocateWith != "" {
			createFn = fmt.Sprintf("create_distributed_table('%s', '%s', colocate_with => '%s')", dist.table, dist.column, dist.colocateWith)
		}
		query := fmt.Sprintf(`
SELECT %s
WHERE NOT EXISTS (
	SELECT 1 FROM pg_dist_partition WHERE logicalrelid = '%s'::regclass
)`, createFn, dist.table)
		if err := w.createCitusDDL(ctx, query, fmt.Sprintf("creating citus distributed table %s", dist.table)); err != nil {
			return err
		}
	}

	citusTables := []string{
		tableItem,
		tableWareHouse,
		tableDistrict,
		tableCustomer,
		tableHistory,
		tableNewOrder,
		tableOrders,
		tableOrderLine,
		tableStock,
	}
	if err := w.checkCitusTables(ctx, citusTables); err != nil {
		return err
	}
	return w.printCitusShardPlacements(ctx, citusTables)
}

func (w *ddlManager) checkCitusWorkers(ctx context.Context) error {
	s := getTPCCState(ctx)

	var workerCount int
	if err := s.Conn.QueryRowContext(ctx, "SELECT count(*) FROM citus_get_active_worker_nodes()").Scan(&workerCount); err != nil {
		return fmt.Errorf("checking citus worker nodes failed: %w", err)
	}
	if workerCount == 0 {
		return fmt.Errorf("citus has no active worker nodes; add workers with citus_add_node before running prepare --citus")
	}
	fmt.Printf("citus active worker nodes: %d\n", workerCount)
	return nil
}

func (w *ddlManager) checkCitusTables(ctx context.Context, tables []string) error {
	s := getTPCCState(ctx)
	regclasses := make([]string, 0, len(tables))
	for _, table := range tables {
		regclasses = append(regclasses, fmt.Sprintf("'%s'::regclass", table))
	}

	rows, err := s.Conn.QueryContext(ctx, fmt.Sprintf(`
SELECT c.relname
FROM pg_dist_partition p
JOIN pg_class c ON c.oid = p.logicalrelid
WHERE p.logicalrelid IN (%s)`, strings.Join(regclasses, ",")))
	if err != nil {
		return fmt.Errorf("checking citus distributed tables failed: %w", err)
	}
	defer rows.Close()

	distributed := make(map[string]bool, len(tables))
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("checking citus distributed tables failed: %w", err)
		}
		distributed[table] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("checking citus distributed tables failed: %w", err)
	}

	missing := make([]string, 0)
	for _, table := range tables {
		if !distributed[table] {
			missing = append(missing, table)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("citus did not register tables as distributed/reference: %s", strings.Join(missing, ", "))
	}

	return nil
}

func (w *ddlManager) printCitusShardPlacements(ctx context.Context, tables []string) error {
	s := getTPCCState(ctx)
	regclasses := make([]string, 0, len(tables))
	for _, table := range tables {
		regclasses = append(regclasses, fmt.Sprintf("'%s'::regclass", table))
	}

	rows, err := s.Conn.QueryContext(ctx, fmt.Sprintf(`
SELECT n.nodename, n.nodeport, count(*) AS placements
FROM pg_dist_shard s
JOIN pg_dist_placement p ON p.shardid = s.shardid
JOIN pg_dist_node n ON n.groupid = p.groupid
WHERE s.logicalrelid IN (%s)
GROUP BY n.nodename, n.nodeport
ORDER BY n.nodename, n.nodeport`, strings.Join(regclasses, ",")))
	if err != nil {
		return fmt.Errorf("checking citus shard placements failed: %w", err)
	}
	defer rows.Close()

	totalPlacements := 0
	for rows.Next() {
		var nodeName string
		var nodePort int
		var placements int
		if err := rows.Scan(&nodeName, &nodePort, &placements); err != nil {
			return fmt.Errorf("checking citus shard placements failed: %w", err)
		}
		totalPlacements += placements
		fmt.Printf("citus shard placements on %s:%d: %d\n", nodeName, nodePort, placements)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("checking citus shard placements failed: %w", err)
	}
	if totalPlacements == 0 {
		return fmt.Errorf("citus did not create shard placements for tpcc tables")
	}

	return nil
}

func (w *ddlManager) dropTable(ctx context.Context) error {
	s := getTPCCState(ctx)
	for _, tbl := range tables {
		fmt.Printf("DROP TABLE IF EXISTS %s\n", tbl)
		if _, err := s.Conn.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tbl)); err != nil {
			return err
		}
	}

	return nil
}
