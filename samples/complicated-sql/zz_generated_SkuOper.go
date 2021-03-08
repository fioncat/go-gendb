// Code generated by go-gendb, DO NOT EDIT.
// go-gendb version: 0.3
// source: samples/complicated-sql/oper.go
package db

import (
	run "github.com/fioncat/go-gendb/api/sql/run"
	sql "database/sql"
	strings "strings"
)

// all sql statement(s) to use
const (
	_SkuOper_Gets              = "SELECT sku.id, ifnull(se.ref_id, '') RefId, sku.status, ifnull(se.origin_price, 0) Price, sku.sku_uid Uid FROM sku LEFT JOIN sku_ezbuy se ON se.sku_id=sku.id JOIN sku_factor sf ON sf.sku_id=sku.id WHERE sku.product_id=? AND (sku.status=1 OR sku.status=2) AND sf.sku_selling_method=2;"
	_SkuOper_GetProps          = "SELECT sai.sku_id, sai.attributes_id AttrId, sai.attributes_values_id ValId, attr.attributes_sort_order AttrSort, val.attributes_values_sort_order ValSort FROM sku_attributes_info sai JOIN `attributes` attr ON attr.id=sai.attributes_id JOIN attributes_values val ON val.id=sai.attributes_values_id WHERE products_id=? AND attr.status=1 AND val.status=1;"
	_SkuOper_GetPropImgs       = "SELECT attr_value_ids_json Pair, images_id ImgId FROM products_attributes_images WHERE products_id=? AND status=1;"
	_SkuOper_GetPropTitles0    = "SELECT ad.attributes_id Id, ad.languages_id LangId, ad.attributes_title Title FROM attributes_desc ad WHERE ad.attributes_id IN ("
	_SkuOper_GetPropTitles1    = "?,"
	_SkuOper_GetPropTitles2    = ") AND languages_id IN (%v);"
	_SkuOper_GetPropValTitles0 = "SELECT avd.attributes_values_id Id, avd.languages_id LangId, avd.attributes_values_title Title FROM attributes_values_desc avd WHERE avd.attributes_values_id IN ("
	_SkuOper_GetPropValTitles1 = "?,"
	_SkuOper_GetPropValTitles2 = ") AND languages_id IN (%v);"
	_SkuOper_SellType          = "SELECT sku_selling_method FROM sku_factor WHERE sku_id=?"
)

var SkuOper = &_SkuOper{}

// Sku is a struct auto generated by SkuOper.Gets
type Sku struct {
	Id     int64   `table:"sku" field:"id"`
	RefId  string  `table:"sku_ezbuy" field:"ref_id"`
	Status int32   `table:"sku" field:"status"`
	Price  float64 `table:"sku_ezbuy" field:"origin_price"`
	Uid    string  `table:"sku" field:"sku_uid"`
}

// SkuProp is a struct auto generated by SkuOper.GetProps
type SkuProp struct {
	SkuId    int64 `table:"sku_attributes_info" field:"sku_id"`
	AttrId   int64 `table:"sku_attributes_info" field:"attributes_id"`
	ValId    int64 `table:"sku_attributes_info" field:"attributes_values_id"`
	AttrSort int32 `table:"attributes" field:"attributes_sort_order"`
	ValSort  int32 `table:"attributes_values" field:"attributes_values_sort_order"`
}

// PropImg is a struct auto generated by SkuOper.GetPropImgs
type PropImg struct {
	Pair  string `table:"products_attributes_images" field:"attr_value_ids_json"`
	ImgId int64  `table:"products_attributes_images" field:"images_id"`
}

// PropTitle is a struct auto generated by SkuOper.GetPropTitles
type PropTitle struct {
	Id     int64  `table:"attributes_desc" field:"attributes_id"`
	LangId int32  `table:"attributes_desc" field:"languages_id"`
	Title  string `table:"attributes_desc" field:"attributes_title"`
}

type _SkuOper struct {}

func (*_SkuOper) Gets(id int64) ([]*Sku, error) {
	var os []*Sku
	err := run.QueryMany(getDB(), _SkuOper_Gets, nil, []interface{}{id}, func(rows *sql.Rows) error {
		o := new(Sku)
		err := rows.Scan(&o.Id, &o.RefId, &o.Status, &o.Price, &o.Uid)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}

func (*_SkuOper) GetProps(id int64) ([]*SkuProp, error) {
	var os []*SkuProp
	err := run.QueryMany(getDB(), _SkuOper_GetProps, nil, []interface{}{id}, func(rows *sql.Rows) error {
		o := new(SkuProp)
		err := rows.Scan(&o.SkuId, &o.AttrId, &o.ValId, &o.AttrSort, &o.ValSort)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}

func (*_SkuOper) GetPropImgs(id int64) ([]*PropImg, error) {
	var os []*PropImg
	err := run.QueryMany(getDB(), _SkuOper_GetPropImgs, nil, []interface{}{id}, func(rows *sql.Rows) error {
		o := new(PropImg)
		err := rows.Scan(&o.Pair, &o.ImgId)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}

func (*_SkuOper) GetPropTitles(attrIds []int64, langCodes string) ([]*PropTitle, error) {
	// [gendb] dynamic start.
	pvs := make([]interface{}, 0, len(attrIds))
	rvs := make([]interface{}, 0, 1)
	slice := make([]string, 0, 2+len(attrIds))
	// concat: part 0
	slice = append(slice, _SkuOper_GetPropTitles0)
	// concat: part 1
	lastidx := len(_SkuOper_GetPropTitles1) - 1
	for i, id := range attrIds {
		pvs = append(pvs, id)
		if i == len(attrIds) - 1 {
			slice = append(slice, _SkuOper_GetPropTitles1[:lastidx])
		} else {
			slice = append(slice, _SkuOper_GetPropTitles1)
		}
	}
	// concat: part 2
	slice = append(slice, _SkuOper_GetPropTitles2)
	rvs = append(rvs, langCodes)
	// concat
	_sql := strings.Join(slice, " ")
	// [gendb] dynamic done.
	var os []*PropTitle
	err := run.QueryMany(getDB(), _sql, rvs, pvs, func(rows *sql.Rows) error {
		o := new(PropTitle)
		err := rows.Scan(&o.Id, &o.LangId, &o.Title)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}

func (*_SkuOper) GetPropValTitles(valIds []int64, langCodes string) ([]*PropTitle, error) {
	// [gendb] dynamic start.
	pvs := make([]interface{}, 0, len(valIds))
	rvs := make([]interface{}, 0, 1)
	slice := make([]string, 0, 2+len(valIds))
	// concat: part 0
	slice = append(slice, _SkuOper_GetPropValTitles0)
	// concat: part 1
	lastidx := len(_SkuOper_GetPropValTitles1) - 1
	for i, id := range valIds {
		pvs = append(pvs, id)
		if i == len(valIds) - 1 {
			slice = append(slice, _SkuOper_GetPropValTitles1[:lastidx])
		} else {
			slice = append(slice, _SkuOper_GetPropValTitles1)
		}
	}
	// concat: part 2
	slice = append(slice, _SkuOper_GetPropValTitles2)
	rvs = append(rvs, langCodes)
	// concat
	_sql := strings.Join(slice, " ")
	// [gendb] dynamic done.
	var os []*PropTitle
	err := run.QueryMany(getDB(), _sql, rvs, pvs, func(rows *sql.Rows) error {
		o := new(PropTitle)
		err := rows.Scan(&o.Id, &o.LangId, &o.Title)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}

func (*_SkuOper) SellType(skuId int64) (int32, error) {
	var o int32
	err := run.QueryOne(getDB(), _SkuOper_SellType, nil, []interface{}{skuId}, func(rows *sql.Rows) error {
		return rows.Scan(&o)
	})
	return o, err
}