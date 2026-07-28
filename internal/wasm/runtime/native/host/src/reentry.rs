use std::marker::PhantomData;
use std::rc::Rc;

pub struct ActiveCall<'a> {
    _lifetime: PhantomData<&'a mut ()>,
    _thread_bound: PhantomData<Rc<()>>,
}
