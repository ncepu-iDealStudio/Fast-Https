// 配置表单：字段结构由 Go 侧 configops 的 fieldSpec 决定，
// 前端只按 type 选控件，不硬编码字段名。
export type FieldType =
  | 'text' | 'number' | 'bool' | 'select' | 'list'
  | 'textarea' | 'email' | 'multiselect' | 'pathlist';

export interface ConfigField {
  path: string;
  label: string;
  group: string;
  type: FieldType;
  value: string;
  options?: string[];
  help?: string;
  missing: boolean;
  browse?: 'dir' | 'file';
}

export interface ConfigForm {
  path: string;
  fields: ConfigField[];
  size: number;
  modified: string;
  writable: boolean;
  preserved: string[];
}
