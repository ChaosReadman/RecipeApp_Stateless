import xml.etree.ElementTree as ET
import sqlite3
import re
import os

# スクリプトの場所を基準にパスを構成（移植性の向上とプライバシー保護）
base_dir = os.path.dirname(os.path.abspath(__file__))
xml_file_path = os.path.join(base_dir, 'nutrient.xml')
db_file_path = os.path.join(base_dir, 'nutrient.db')

def clean_value(value_str):
    """
    XMLから抽出した文字列値を数値に変換する。
    'Tr', '-', '[数値]' などの特殊な形式を処理する。
    """
    if value_str is None:
        return None
    value_str = value_str.strip()
    if value_str.lower() == 'tr' or value_str == '-':
        return None
    # '[数値]' 形式から数値を抽出
    match = re.match(r'\[(.*?)\]', value_str)
    if match:
        try:
            return float(match.group(1))
        except ValueError:
            return None
    try:
        return float(value_str)
    except ValueError:
        return None

def parse_xml_and_extract_data(xml_path):
    """
    nutrient.xmlをパースし、食品データを抽出する。
    """
    tree = ET.parse(xml_path)
    root = tree.getroot()
    
    foods_data = []
    
    # 全ての栄養素タグ名を収集（スキーマ定義のため）
    nutrient_tags = set()
    for food_elem in root.findall('FOOD'):
        for child in food_elem:
            # ネームスペースを除去してタグ名を取得
            tag_name = child.tag.split('}')[-1]
            if tag_name not in ['ADDITIONAL']: # ADDITIONALは別途処理
                nutrient_tags.add(tag_name.lower().replace('-', '_')) # SQLiteの予約語やハイフンを考慮
    
    # 栄養素の順序を固定するためにソート
    sorted_nutrient_tags = sorted(list(nutrient_tags))

    for food_elem in root.findall('FOOD'):
        food_info = {
            'group_id': food_elem.get('{http://example.com/fs}GROUP'),
            'num_id': food_elem.get('{http://example.com/fs}NUM'),
            'food_id': food_elem.get('{http://example.com/fs}ID'),
            'name': food_elem.get('{http://example.com/fs}NAME')
        }
        
        # 栄養素の値を初期化
        for tag in sorted_nutrient_tags:
            food_info[tag] = None
            
        additional_info = ""

        for child in food_elem:
            tag_name = child.tag.split('}')[-1]
            if tag_name == 'ADDITIONAL':
                additional_info = child.text.strip() if child.text else ""
            else:
                attr_value = child.get('{http://example.com/fs}VALUE')
                food_info[tag_name.lower().replace('-', '_')] = clean_value(attr_value)
        
        food_info['additional'] = additional_info
        foods_data.append(food_info)
        
    return foods_data, sorted_nutrient_tags

def create_table(cursor, nutrient_tags):
    """
    SQLiteデータベースにテーブルを作成する。
    """
    # 基本カラム
    columns = [
        'group_id TEXT',
        'num_id TEXT',
        'food_id TEXT PRIMARY KEY',
        'name TEXT'
    ]
    
    # 栄養素カラムを追加
    for tag in nutrient_tags:
        columns.append(f'{tag} REAL') # 数値としてREAL型を使用
    
    # ADDITIONALカラムを追加
    columns.append('additional TEXT')

    create_table_sql = f"""
    CREATE TABLE IF NOT EXISTS foods (
        {', '.join(columns)}
    )
    """
    cursor.execute(create_table_sql)

    

def insert_data(cursor, foods_data, nutrient_tags):
    """
    抽出したデータをSQLiteデータベースに挿入する。
    """
    # カラム名のリストを生成（food_id, name, nutrient_tags, additionalの順）
    all_columns = ['group_id', 'num_id', 'food_id', 'name'] + nutrient_tags + ['additional']
    
    # プレースホルダのリストを生成
    placeholders = ', '.join(['?' for _ in all_columns])
    
    insert_sql = f"""
    INSERT INTO foods ({', '.join(all_columns)})
    VALUES ({placeholders})
    """

    # リスト内包表記で一括データを作成し、executemanyで高速挿入
    data_tuples = [[item.get(col) for col in all_columns] for item in foods_data]
    cursor.executemany(insert_sql, data_tuples)

def main():
    conn = None
    try:
        conn = sqlite3.connect(db_file_path)
        cursor = conn.cursor()
        
        print(f"XMLファイルをパース中: {xml_file_path}")
        foods_data, nutrient_tags = parse_xml_and_extract_data(xml_file_path)
        print(f"抽出された食品数: {len(foods_data)}")
        print(f"検出された栄養素タグ: {len(nutrient_tags)}種類")

        print("テーブルを作成中...")
        create_table(cursor, nutrient_tags)
        print("テーブル作成完了。")

        # 食品マスターデータのみをリフレッシュ（ユーザーデータは残す）
        print("既存の食品データをクリア中...")
        cursor.execute("DELETE FROM foods")

        print("データを挿入中...")
        insert_data(cursor, foods_data, nutrient_tags)
        conn.commit()
        print("データ挿入完了。")
        
        print(f"データが正常に {db_file_path} に保存されました。")

        # 確認のためにいくつかデータを取得
        cursor.execute("SELECT food_id, name, enerc_kcal, prot_ FROM foods LIMIT 5")
        print("\n--- 挿入されたデータの例 ---")
        for row in cursor.fetchall():
            print(row)

    except FileNotFoundError:
        print(f"エラー: ファイルが見つかりません - {xml_file_path}")
    except ET.ParseError as e:
        print(f"エラー: XMLパースエラー - {e}")
    except sqlite3.Error as e:
        print(f"エラー: SQLite操作エラー - {e}")
    finally:
        if conn:
            conn.close()

if __name__ == '__main__':
    main()