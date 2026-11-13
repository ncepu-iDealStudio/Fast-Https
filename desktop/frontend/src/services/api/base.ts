// Go Binding 的唯一接入点：把生成物断言成精确契约。
// 除这一处外，业务代码不 import bindings，也不出现 any。
import { DesktopService as Bindings } from '../../../bindings/fast-https-desktop';
import type { DesktopServiceContract } from './types';

export const service = Bindings as unknown as DesktopServiceContract;

/** 把 Wails / CLI 异常翻成可展示的人话。 */
export function messageOf(error: unknown): string {
  if (error === null || error === undefined) return '';
  if (typeof error === 'string') return error;
  if (error instanceof Error) return error.message;
  if (typeof error === 'object' && 'message' in error) {
    return String((error as { message: unknown }).message);
  }
  return String(error);
}
