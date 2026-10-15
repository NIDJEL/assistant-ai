import { render, screen } from '@testing-library/react';
import { NotificationSettings } from './NotificationSettings';
import { registerNotifications } from './notifications';
import { getDevices, getPushConfig, pushCapability } from './push';
vi.mock('../../app/AuthContext',()=>({useAuth:()=>({user:{id:'alice'}})}));
vi.mock('./notifications',()=>({registerNotifications:vi.fn()}));
vi.mock('./push',async original=>({...await original<object>(),getDevices:vi.fn(),getPushConfig:vi.fn(),pushCapability:vi.fn()}));
beforeEach(()=>{
  vi.stubGlobal('Notification',{permission:'default'});
  vi.mocked(pushCapability).mockReturnValue('supported');vi.mocked(getPushConfig).mockResolvedValue({enabled:true,public_key:'key'});
  vi.mocked(getDevices).mockResolvedValue({subscriptions:[]});vi.mocked(registerNotifications).mockResolvedValue(undefined);
});
it('shows notifications off while permission is still default',async()=>{render(<NotificationSettings/>);expect(await screen.findByText('Уведомления выключены')).toBeVisible();expect(await screen.findByRole('button',{name:'Включить уведомления'})).toBeVisible();});
it('explains denied permission and disables the enable button',async()=>{vi.stubGlobal('Notification',{permission:'denied'});render(<NotificationSettings/>);expect(await screen.findByText('Браузер запретил уведомления')).toBeVisible();expect(screen.getByRole('button',{name:'Включить уведомления'})).toBeDisabled();});
it('explains unsupported browsers without pretending background delivery works',async()=>{vi.mocked(pushCapability).mockReturnValue('unsupported');render(<NotificationSettings/>);expect(await screen.findByText(/Этот браузер не поддерживает фоновые/)).toBeVisible();expect(screen.queryByRole('button',{name:'Включить уведомления'})).not.toBeInTheDocument();});
it('shows the four iPhone installation steps',async()=>{vi.mocked(pushCapability).mockReturnValue('ios-install');render(<NotificationSettings/>);expect(await screen.findByText('Выберите «На экран Домой».')).toBeVisible();expect(screen.getAllByRole('listitem')).toHaveLength(4);});
it('shows an active current device and a test button only when registered',async()=>{
  vi.stubGlobal('Notification',{permission:'granted'});localStorage.setItem('gena.push.device.alice','device');
  vi.mocked(registerNotifications).mockResolvedValue({pushManager:{getSubscription:async()=>({})}} as unknown as ServiceWorkerRegistration);
  vi.mocked(getDevices).mockResolvedValue({subscriptions:[{id:'device',platform:'iPhone',disabled_at:null,last_success_at:null,failure_count:0}]});
  render(<NotificationSettings/>);expect(await screen.findByText('Уведомления включены')).toBeVisible();expect(screen.getByRole('button',{name:'Отправить тестовое уведомление'})).toBeVisible();expect(screen.getByText(/iPhone — этот браузер/)).toBeVisible();
});
it('shows a server configuration limitation',async()=>{vi.mocked(getPushConfig).mockResolvedValue({enabled:false,public_key:''});render(<NotificationSettings/>);expect(await screen.findByText(/пока не настроены на сервере/)).toBeVisible();expect(screen.getByRole('button',{name:'Включить уведомления'})).toBeDisabled();});
